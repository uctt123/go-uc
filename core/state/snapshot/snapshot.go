
















package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/trie"
)

var (
	snapshotCleanAccountHitMeter   = metrics.NewRegisteredMeter("state/snapshot/clean/account/hit", nil)
	snapshotCleanAccountMissMeter  = metrics.NewRegisteredMeter("state/snapshot/clean/account/miss", nil)
	snapshotCleanAccountInexMeter  = metrics.NewRegisteredMeter("state/snapshot/clean/account/inex", nil)
	snapshotCleanAccountReadMeter  = metrics.NewRegisteredMeter("state/snapshot/clean/account/read", nil)
	snapshotCleanAccountWriteMeter = metrics.NewRegisteredMeter("state/snapshot/clean/account/write", nil)

	snapshotCleanStorageHitMeter   = metrics.NewRegisteredMeter("state/snapshot/clean/storage/hit", nil)
	snapshotCleanStorageMissMeter  = metrics.NewRegisteredMeter("state/snapshot/clean/storage/miss", nil)
	snapshotCleanStorageInexMeter  = metrics.NewRegisteredMeter("state/snapshot/clean/storage/inex", nil)
	snapshotCleanStorageReadMeter  = metrics.NewRegisteredMeter("state/snapshot/clean/storage/read", nil)
	snapshotCleanStorageWriteMeter = metrics.NewRegisteredMeter("state/snapshot/clean/storage/write", nil)

	snapshotDirtyAccountHitMeter   = metrics.NewRegisteredMeter("state/snapshot/dirty/account/hit", nil)
	snapshotDirtyAccountMissMeter  = metrics.NewRegisteredMeter("state/snapshot/dirty/account/miss", nil)
	snapshotDirtyAccountInexMeter  = metrics.NewRegisteredMeter("state/snapshot/dirty/account/inex", nil)
	snapshotDirtyAccountReadMeter  = metrics.NewRegisteredMeter("state/snapshot/dirty/account/read", nil)
	snapshotDirtyAccountWriteMeter = metrics.NewRegisteredMeter("state/snapshot/dirty/account/write", nil)

	snapshotDirtyStorageHitMeter   = metrics.NewRegisteredMeter("state/snapshot/dirty/storage/hit", nil)
	snapshotDirtyStorageMissMeter  = metrics.NewRegisteredMeter("state/snapshot/dirty/storage/miss", nil)
	snapshotDirtyStorageInexMeter  = metrics.NewRegisteredMeter("state/snapshot/dirty/storage/inex", nil)
	snapshotDirtyStorageReadMeter  = metrics.NewRegisteredMeter("state/snapshot/dirty/storage/read", nil)
	snapshotDirtyStorageWriteMeter = metrics.NewRegisteredMeter("state/snapshot/dirty/storage/write", nil)

	snapshotDirtyAccountHitDepthHist = metrics.NewRegisteredHistogram("state/snapshot/dirty/account/hit/depth", nil, metrics.NewExpDecaySample(1028, 0.015))
	snapshotDirtyStorageHitDepthHist = metrics.NewRegisteredHistogram("state/snapshot/dirty/storage/hit/depth", nil, metrics.NewExpDecaySample(1028, 0.015))

	snapshotFlushAccountItemMeter = metrics.NewRegisteredMeter("state/snapshot/flush/account/item", nil)
	snapshotFlushAccountSizeMeter = metrics.NewRegisteredMeter("state/snapshot/flush/account/size", nil)
	snapshotFlushStorageItemMeter = metrics.NewRegisteredMeter("state/snapshot/flush/storage/item", nil)
	snapshotFlushStorageSizeMeter = metrics.NewRegisteredMeter("state/snapshot/flush/storage/size", nil)

	snapshotBloomIndexTimer = metrics.NewRegisteredResettingTimer("state/snapshot/bloom/index", nil)
	snapshotBloomErrorGauge = metrics.NewRegisteredGaugeFloat64("state/snapshot/bloom/error", nil)

	snapshotBloomAccountTrueHitMeter  = metrics.NewRegisteredMeter("state/snapshot/bloom/account/truehit", nil)
	snapshotBloomAccountFalseHitMeter = metrics.NewRegisteredMeter("state/snapshot/bloom/account/falsehit", nil)
	snapshotBloomAccountMissMeter     = metrics.NewRegisteredMeter("state/snapshot/bloom/account/miss", nil)

	snapshotBloomStorageTrueHitMeter  = metrics.NewRegisteredMeter("state/snapshot/bloom/storage/truehit", nil)
	snapshotBloomStorageFalseHitMeter = metrics.NewRegisteredMeter("state/snapshot/bloom/storage/falsehit", nil)
	snapshotBloomStorageMissMeter     = metrics.NewRegisteredMeter("state/snapshot/bloom/storage/miss", nil)

	
	
	
	ErrSnapshotStale = errors.New("snapshot stale")

	
	
	
	ErrNotCoveredYet = errors.New("not covered yet")

	
	
	errSnapshotCycle = errors.New("snapshot cycle")
)


type Snapshot interface {
	
	Root() common.Hash

	
	
	Account(hash common.Hash) (*Account, error)

	
	
	AccountRLP(hash common.Hash) ([]byte, error)

	
	
	Storage(accountHash, storageHash common.Hash) ([]byte, error)
}



type snapshot interface {
	Snapshot

	
	
	
	
	
	Parent() snapshot

	
	
	
	
	Update(blockRoot common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storage map[common.Hash]map[common.Hash][]byte) *diffLayer

	
	
	
	Journal(buffer *bytes.Buffer) (common.Hash, error)

	
	
	Stale() bool

	
	AccountIterator(seek common.Hash) AccountIterator

	
	StorageIterator(account common.Hash, seek common.Hash) (StorageIterator, bool)
}










type Tree struct {
	diskdb ethdb.KeyValueStore      
	triedb *trie.Database           
	cache  int                      
	layers map[common.Hash]snapshot 
	lock   sync.RWMutex
}








func New(diskdb ethdb.KeyValueStore, triedb *trie.Database, cache int, root common.Hash, async bool) *Tree {
	
	snap := &Tree{
		diskdb: diskdb,
		triedb: triedb,
		cache:  cache,
		layers: make(map[common.Hash]snapshot),
	}
	if !async {
		defer snap.waitBuild()
	}
	
	head, err := loadSnapshot(diskdb, triedb, cache, root)
	if err != nil {
		log.Warn("Failed to load snapshot, regenerating", "err", err)
		snap.Rebuild(root)
		return snap
	}
	
	for head != nil {
		snap.layers[head.Root()] = head
		head = head.Parent()
	}
	return snap
}



func (t *Tree) waitBuild() {
	
	var done chan struct{}

	t.lock.RLock()
	for _, layer := range t.layers {
		if layer, ok := layer.(*diskLayer); ok {
			done = layer.genPending
			break
		}
	}
	t.lock.RUnlock()

	
	if done != nil {
		<-done
	}
}



func (t *Tree) Snapshot(blockRoot common.Hash) Snapshot {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.layers[blockRoot]
}



func (t *Tree) Update(blockRoot common.Hash, parentRoot common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storage map[common.Hash]map[common.Hash][]byte) error {
	
	
	
	
	
	
	if blockRoot == parentRoot {
		return errSnapshotCycle
	}
	
	parent := t.Snapshot(parentRoot).(snapshot)
	if parent == nil {
		return fmt.Errorf("parent [%#x] snapshot missing", parentRoot)
	}
	snap := parent.Update(blockRoot, destructs, accounts, storage)

	
	t.lock.Lock()
	defer t.lock.Unlock()

	t.layers[snap.root] = snap
	return nil
}




func (t *Tree) Cap(root common.Hash, layers int) error {
	
	snap := t.Snapshot(root)
	if snap == nil {
		return fmt.Errorf("snapshot [%#x] missing", root)
	}
	diff, ok := snap.(*diffLayer)
	if !ok {
		return fmt.Errorf("snapshot [%#x] is disk layer", root)
	}
	
	diff.origin.lock.RLock()
	if diff.origin.genMarker != nil && layers > 8 {
		layers = 8
	}
	diff.origin.lock.RUnlock()

	
	t.lock.Lock()
	defer t.lock.Unlock()

	
	
	
	var persisted *diskLayer

	switch layers {
	case 0:
		
		diff.lock.RLock()
		base := diffToDisk(diff.flatten().(*diffLayer))
		diff.lock.RUnlock()

		
		t.layers = map[common.Hash]snapshot{base.root: base}
		return nil

	case 1:
		
		
		var (
			bottom *diffLayer
			base   *diskLayer
		)
		diff.lock.RLock()
		bottom = diff.flatten().(*diffLayer)
		if bottom.memory >= aggregatorMemoryLimit {
			base = diffToDisk(bottom)
		}
		diff.lock.RUnlock()

		
		if base != nil {
			t.layers = map[common.Hash]snapshot{base.root: base}
			return nil
		}
		
		t.layers[bottom.root] = bottom

	default:
		
		persisted = t.cap(diff, layers)
	}
	
	children := make(map[common.Hash][]common.Hash)
	for root, snap := range t.layers {
		if diff, ok := snap.(*diffLayer); ok {
			parent := diff.parent.Root()
			children[parent] = append(children[parent], root)
		}
	}
	var remove func(root common.Hash)
	remove = func(root common.Hash) {
		delete(t.layers, root)
		for _, child := range children[root] {
			remove(child)
		}
		delete(children, root)
	}
	for root, snap := range t.layers {
		if snap.Stale() {
			remove(root)
		}
	}
	
	if persisted != nil {
		var rebloom func(root common.Hash)
		rebloom = func(root common.Hash) {
			if diff, ok := t.layers[root].(*diffLayer); ok {
				diff.rebloom(persisted)
			}
			for _, child := range children[root] {
				rebloom(child)
			}
		}
		rebloom(persisted.root)
	}
	return nil
}






func (t *Tree) cap(diff *diffLayer, layers int) *diskLayer {
	
	for ; layers > 2; layers-- {
		
		if parent, ok := diff.parent.(*diffLayer); ok {
			diff = parent
		} else {
			
			return nil
		}
	}
	
	
	switch parent := diff.parent.(type) {
	case *diskLayer:
		return nil

	case *diffLayer:
		
		
		flattened := parent.flatten().(*diffLayer)
		t.layers[flattened.root] = flattened

		diff.lock.Lock()
		defer diff.lock.Unlock()

		diff.parent = flattened
		if flattened.memory < aggregatorMemoryLimit {
			
			
			
			
			if flattened.parent.(*diskLayer).genAbort == nil {
				return nil
			}
		}
	default:
		panic(fmt.Sprintf("unknown data layer: %T", parent))
	}
	
	bottom := diff.parent.(*diffLayer)

	bottom.lock.RLock()
	base := diffToDisk(bottom)
	bottom.lock.RUnlock()

	t.layers[base.root] = base
	diff.parent = base
	return base
}



func diffToDisk(bottom *diffLayer) *diskLayer {
	var (
		base  = bottom.parent.(*diskLayer)
		batch = base.diskdb.NewBatch()
		stats *generatorStats
	)
	
	if base.genAbort != nil {
		abort := make(chan *generatorStats)
		base.genAbort <- abort
		stats = <-abort
	}
	
	
	rawdb.DeleteSnapshotRoot(batch)

	
	base.lock.Lock()
	if base.stale {
		panic("parent disk layer is stale") 
	}
	base.stale = true
	base.lock.Unlock()

	
	for hash := range bottom.destructSet {
		
		if base.genMarker != nil && bytes.Compare(hash[:], base.genMarker) > 0 {
			continue
		}
		
		rawdb.DeleteAccountSnapshot(batch, hash)
		base.cache.Set(hash[:], nil)

		it := rawdb.IterateStorageSnapshots(base.diskdb, hash)
		for it.Next() {
			if key := it.Key(); len(key) == 65 { 
				batch.Delete(key)
				base.cache.Del(key[1:])

				snapshotFlushStorageItemMeter.Mark(1)
			}
		}
		it.Release()
	}
	
	for hash, data := range bottom.accountData {
		
		if base.genMarker != nil && bytes.Compare(hash[:], base.genMarker) > 0 {
			continue
		}
		
		rawdb.WriteAccountSnapshot(batch, hash, data)
		base.cache.Set(hash[:], data)
		snapshotCleanAccountWriteMeter.Mark(int64(len(data)))

		if batch.ValueSize() > ethdb.IdealBatchSize {
			if err := batch.Write(); err != nil {
				log.Crit("Failed to write account snapshot", "err", err)
			}
			batch.Reset()
		}
		snapshotFlushAccountItemMeter.Mark(1)
		snapshotFlushAccountSizeMeter.Mark(int64(len(data)))
	}
	
	for accountHash, storage := range bottom.storageData {
		
		if base.genMarker != nil && bytes.Compare(accountHash[:], base.genMarker) > 0 {
			continue
		}
		
		midAccount := base.genMarker != nil && bytes.Equal(accountHash[:], base.genMarker[:common.HashLength])

		for storageHash, data := range storage {
			
			if midAccount && bytes.Compare(storageHash[:], base.genMarker[common.HashLength:]) > 0 {
				continue
			}
			if len(data) > 0 {
				rawdb.WriteStorageSnapshot(batch, accountHash, storageHash, data)
				base.cache.Set(append(accountHash[:], storageHash[:]...), data)
				snapshotCleanStorageWriteMeter.Mark(int64(len(data)))
			} else {
				rawdb.DeleteStorageSnapshot(batch, accountHash, storageHash)
				base.cache.Set(append(accountHash[:], storageHash[:]...), nil)
			}
			snapshotFlushStorageItemMeter.Mark(1)
			snapshotFlushStorageSizeMeter.Mark(int64(len(data)))
		}
		if batch.ValueSize() > ethdb.IdealBatchSize {
			if err := batch.Write(); err != nil {
				log.Crit("Failed to write storage snapshot", "err", err)
			}
			batch.Reset()
		}
	}
	
	rawdb.WriteSnapshotRoot(batch, bottom.root)
	if err := batch.Write(); err != nil {
		log.Crit("Failed to write leftover snapshot", "err", err)
	}
	res := &diskLayer{
		root:       bottom.root,
		cache:      base.cache,
		diskdb:     base.diskdb,
		triedb:     base.triedb,
		genMarker:  base.genMarker,
		genPending: base.genPending,
	}
	
	
	
	
	
	if base.genMarker != nil && base.genAbort != nil {
		res.genMarker = base.genMarker
		res.genAbort = make(chan chan *generatorStats)
		go res.generate(stats)
	}
	return res
}







func (t *Tree) Journal(root common.Hash) (common.Hash, error) {
	
	snap := t.Snapshot(root)
	if snap == nil {
		return common.Hash{}, fmt.Errorf("snapshot [%#x] missing", root)
	}
	
	t.lock.Lock()
	defer t.lock.Unlock()

	journal := new(bytes.Buffer)
	base, err := snap.(snapshot).Journal(journal)
	if err != nil {
		return common.Hash{}, err
	}
	
	rawdb.WriteSnapshotJournal(t.diskdb, journal.Bytes())
	return base, nil
}




func (t *Tree) Rebuild(root common.Hash) {
	t.lock.Lock()
	defer t.lock.Unlock()

	
	var wiper chan struct{}

	
	for _, layer := range t.layers {
		switch layer := layer.(type) {
		case *diskLayer:
			
			if layer.genAbort != nil {
				abort := make(chan *generatorStats)
				layer.genAbort <- abort

				if stats := <-abort; stats != nil {
					wiper = stats.wiping
				}
			}
			
			layer.lock.Lock()
			layer.stale = true
			layer.lock.Unlock()

		case *diffLayer:
			
			layer.lock.Lock()
			atomic.StoreUint32(&layer.stale, 1)
			layer.lock.Unlock()

		default:
			panic(fmt.Sprintf("unknown layer type: %T", layer))
		}
	}
	
	
	log.Info("Rebuilding state snapshot")
	t.layers = map[common.Hash]snapshot{
		root: generateSnapshot(t.diskdb, t.triedb, t.cache, root, wiper),
	}
}



func (t *Tree) AccountIterator(root common.Hash, seek common.Hash) (AccountIterator, error) {
	return newFastAccountIterator(t, root, seek)
}



func (t *Tree) StorageIterator(root common.Hash, account common.Hash, seek common.Hash) (StorageIterator, error) {
	return newFastStorageIterator(t, root, account, seek)
}
