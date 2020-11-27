















package snapshot

import (
	"bytes"
	"sync"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)


type diskLayer struct {
	diskdb ethdb.KeyValueStore 
	triedb *trie.Database      
	cache  *fastcache.Cache    

	root  common.Hash 
	stale bool        

	genMarker  []byte                    
	genPending chan struct{}             
	genAbort   chan chan *generatorStats 

	lock sync.RWMutex
}


func (dl *diskLayer) Root() common.Hash {
	return dl.root
}


func (dl *diskLayer) Parent() snapshot {
	return nil
}



func (dl *diskLayer) Stale() bool {
	dl.lock.RLock()
	defer dl.lock.RUnlock()

	return dl.stale
}



func (dl *diskLayer) Account(hash common.Hash) (*Account, error) {
	data, err := dl.AccountRLP(hash)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 { 
		return nil, nil
	}
	account := new(Account)
	if err := rlp.DecodeBytes(data, account); err != nil {
		panic(err)
	}
	return account, nil
}



func (dl *diskLayer) AccountRLP(hash common.Hash) ([]byte, error) {
	dl.lock.RLock()
	defer dl.lock.RUnlock()

	
	
	if dl.stale {
		return nil, ErrSnapshotStale
	}
	
	
	if dl.genMarker != nil && bytes.Compare(hash[:], dl.genMarker) > 0 {
		return nil, ErrNotCoveredYet
	}
	
	snapshotDirtyAccountMissMeter.Mark(1)

	
	if blob, found := dl.cache.HasGet(nil, hash[:]); found {
		snapshotCleanAccountHitMeter.Mark(1)
		snapshotCleanAccountReadMeter.Mark(int64(len(blob)))
		return blob, nil
	}
	
	blob := rawdb.ReadAccountSnapshot(dl.diskdb, hash)
	dl.cache.Set(hash[:], blob)

	snapshotCleanAccountMissMeter.Mark(1)
	if n := len(blob); n > 0 {
		snapshotCleanAccountWriteMeter.Mark(int64(n))
	} else {
		snapshotCleanAccountInexMeter.Mark(1)
	}
	return blob, nil
}



func (dl *diskLayer) Storage(accountHash, storageHash common.Hash) ([]byte, error) {
	dl.lock.RLock()
	defer dl.lock.RUnlock()

	
	
	if dl.stale {
		return nil, ErrSnapshotStale
	}
	key := append(accountHash[:], storageHash[:]...)

	
	
	if dl.genMarker != nil && bytes.Compare(key, dl.genMarker) > 0 {
		return nil, ErrNotCoveredYet
	}
	
	snapshotDirtyStorageMissMeter.Mark(1)

	
	if blob, found := dl.cache.HasGet(nil, key); found {
		snapshotCleanStorageHitMeter.Mark(1)
		snapshotCleanStorageReadMeter.Mark(int64(len(blob)))
		return blob, nil
	}
	
	blob := rawdb.ReadStorageSnapshot(dl.diskdb, accountHash, storageHash)
	dl.cache.Set(key, blob)

	snapshotCleanStorageMissMeter.Mark(1)
	if n := len(blob); n > 0 {
		snapshotCleanStorageWriteMeter.Mark(int64(n))
	} else {
		snapshotCleanStorageInexMeter.Mark(1)
	}
	return blob, nil
}




func (dl *diskLayer) Update(blockHash common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storage map[common.Hash]map[common.Hash][]byte) *diffLayer {
	return newDiffLayer(dl, blockHash, destructs, accounts, storage)
}
