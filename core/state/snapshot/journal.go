















package snapshot

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)


type journalGenerator struct {
	Wiping   bool 
	Done     bool 
	Marker   []byte
	Accounts uint64
	Slots    uint64
	Storage  uint64
}


type journalDestruct struct {
	Hash common.Hash
}


type journalAccount struct {
	Hash common.Hash
	Blob []byte
}


type journalStorage struct {
	Hash common.Hash
	Keys []common.Hash
	Vals [][]byte
}


func loadSnapshot(diskdb ethdb.KeyValueStore, triedb *trie.Database, cache int, root common.Hash) (snapshot, error) {
	
	
	baseRoot := rawdb.ReadSnapshotRoot(diskdb)
	if baseRoot == (common.Hash{}) {
		return nil, errors.New("missing or corrupted snapshot")
	}
	base := &diskLayer{
		diskdb: diskdb,
		triedb: triedb,
		cache:  fastcache.New(cache * 1024 * 1024),
		root:   baseRoot,
	}
	
	
	journal := rawdb.ReadSnapshotJournal(diskdb)
	if len(journal) == 0 {
		return nil, errors.New("missing or corrupted snapshot journal")
	}
	r := rlp.NewStream(bytes.NewReader(journal), 0)

	
	var generator journalGenerator
	if err := r.Decode(&generator); err != nil {
		return nil, fmt.Errorf("failed to load snapshot progress marker: %v", err)
	}
	
	snapshot, err := loadDiffLayer(base, r)
	if err != nil {
		return nil, err
	}
	
	
	if head := snapshot.Root(); head != root {
		return nil, fmt.Errorf("head doesn't match snapshot: have %#x, want %#x", head, root)
	}
	
	if !generator.Done {
		
		
		
		var wiper chan struct{}
		if generator.Wiping {
			log.Info("Resuming previous snapshot wipe")
			wiper = wipeSnapshot(diskdb, false)
		}
		
		base.genMarker = generator.Marker
		if base.genMarker == nil {
			base.genMarker = []byte{}
		}
		base.genPending = make(chan struct{})
		base.genAbort = make(chan chan *generatorStats)

		var origin uint64
		if len(generator.Marker) >= 8 {
			origin = binary.BigEndian.Uint64(generator.Marker)
		}
		go base.generate(&generatorStats{
			wiping:   wiper,
			origin:   origin,
			start:    time.Now(),
			accounts: generator.Accounts,
			slots:    generator.Slots,
			storage:  common.StorageSize(generator.Storage),
		})
	}
	return snapshot, nil
}



func loadDiffLayer(parent snapshot, r *rlp.Stream) (snapshot, error) {
	
	var root common.Hash
	if err := r.Decode(&root); err != nil {
		
		if err == io.EOF {
			return parent, nil
		}
		return nil, fmt.Errorf("load diff root: %v", err)
	}
	var destructs []journalDestruct
	if err := r.Decode(&destructs); err != nil {
		return nil, fmt.Errorf("load diff destructs: %v", err)
	}
	destructSet := make(map[common.Hash]struct{})
	for _, entry := range destructs {
		destructSet[entry.Hash] = struct{}{}
	}
	var accounts []journalAccount
	if err := r.Decode(&accounts); err != nil {
		return nil, fmt.Errorf("load diff accounts: %v", err)
	}
	accountData := make(map[common.Hash][]byte)
	for _, entry := range accounts {
		if len(entry.Blob) > 0 { 
			accountData[entry.Hash] = entry.Blob
		} else {
			accountData[entry.Hash] = nil
		}
	}
	var storage []journalStorage
	if err := r.Decode(&storage); err != nil {
		return nil, fmt.Errorf("load diff storage: %v", err)
	}
	storageData := make(map[common.Hash]map[common.Hash][]byte)
	for _, entry := range storage {
		slots := make(map[common.Hash][]byte)
		for i, key := range entry.Keys {
			if len(entry.Vals[i]) > 0 { 
				slots[key] = entry.Vals[i]
			} else {
				slots[key] = nil
			}
		}
		storageData[entry.Hash] = slots
	}
	return loadDiffLayer(newDiffLayer(parent, root, destructSet, accountData, storageData), r)
}



func (dl *diskLayer) Journal(buffer *bytes.Buffer) (common.Hash, error) {
	
	var stats *generatorStats
	if dl.genAbort != nil {
		abort := make(chan *generatorStats)
		dl.genAbort <- abort

		if stats = <-abort; stats != nil {
			stats.Log("Journalling in-progress snapshot", dl.root, dl.genMarker)
		}
	}
	
	dl.lock.RLock()
	defer dl.lock.RUnlock()

	if dl.stale {
		return common.Hash{}, ErrSnapshotStale
	}
	
	entry := journalGenerator{
		Done:   dl.genMarker == nil,
		Marker: dl.genMarker,
	}
	if stats != nil {
		entry.Wiping = (stats.wiping != nil)
		entry.Accounts = stats.accounts
		entry.Slots = stats.slots
		entry.Storage = uint64(stats.storage)
	}
	if err := rlp.Encode(buffer, entry); err != nil {
		return common.Hash{}, err
	}
	return dl.root, nil
}



func (dl *diffLayer) Journal(buffer *bytes.Buffer) (common.Hash, error) {
	
	base, err := dl.parent.Journal(buffer)
	if err != nil {
		return common.Hash{}, err
	}
	
	dl.lock.RLock()
	defer dl.lock.RUnlock()

	if dl.Stale() {
		return common.Hash{}, ErrSnapshotStale
	}
	
	if err := rlp.Encode(buffer, dl.root); err != nil {
		return common.Hash{}, err
	}
	destructs := make([]journalDestruct, 0, len(dl.destructSet))
	for hash := range dl.destructSet {
		destructs = append(destructs, journalDestruct{Hash: hash})
	}
	if err := rlp.Encode(buffer, destructs); err != nil {
		return common.Hash{}, err
	}
	accounts := make([]journalAccount, 0, len(dl.accountData))
	for hash, blob := range dl.accountData {
		accounts = append(accounts, journalAccount{Hash: hash, Blob: blob})
	}
	if err := rlp.Encode(buffer, accounts); err != nil {
		return common.Hash{}, err
	}
	storage := make([]journalStorage, 0, len(dl.storageData))
	for hash, slots := range dl.storageData {
		keys := make([]common.Hash, 0, len(slots))
		vals := make([][]byte, 0, len(slots))
		for key, val := range slots {
			keys = append(keys, key)
			vals = append(vals, val)
		}
		storage = append(storage, journalStorage{Hash: hash, Keys: keys, Vals: vals})
	}
	if err := rlp.Encode(buffer, storage); err != nil {
		return common.Hash{}, err
	}
	return base, nil
}
