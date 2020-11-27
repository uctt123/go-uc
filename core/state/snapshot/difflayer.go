















package snapshot

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/steakknife/bloomfilter"
)

var (
	
	
	
	
	
	
	
	aggregatorMemoryLimit = uint64(4 * 1024 * 1024)

	
	
	
	
	
	
	aggregatorItemLimit = aggregatorMemoryLimit / 42

	
	
	
	
	
	
	
	bloomTargetError = 0.02

	
	
	bloomSize = math.Ceil(float64(aggregatorItemLimit) * math.Log(bloomTargetError) / math.Log(1/math.Pow(2, math.Log(2))))

	
	
	
	bloomFuncs = math.Round((bloomSize / float64(aggregatorItemLimit)) * math.Log(2))

	
	
	
	
	
	bloomDestructHasherOffset = 0
	bloomAccountHasherOffset  = 0
	bloomStorageHasherOffset  = 0
)

func init() {
	
	bloomDestructHasherOffset = rand.Intn(25)
	bloomAccountHasherOffset = rand.Intn(25)
	bloomStorageHasherOffset = rand.Intn(25)

	
	
	
	for bloomAccountHasherOffset == bloomDestructHasherOffset {
		bloomAccountHasherOffset = rand.Intn(25)
	}
}







type diffLayer struct {
	origin *diskLayer 
	parent snapshot   
	memory uint64     

	root  common.Hash 
	stale uint32      

	
	
	
	
	
	
	
	destructSet map[common.Hash]struct{}               
	accountList []common.Hash                          
	accountData map[common.Hash][]byte                 
	storageList map[common.Hash][]common.Hash          
	storageData map[common.Hash]map[common.Hash][]byte 

	diffed *bloomfilter.Filter 

	lock sync.RWMutex
}




type destructBloomHasher common.Hash

func (h destructBloomHasher) Write(p []byte) (n int, err error) { panic("not implemented") }
func (h destructBloomHasher) Sum(b []byte) []byte               { panic("not implemented") }
func (h destructBloomHasher) Reset()                            { panic("not implemented") }
func (h destructBloomHasher) BlockSize() int                    { panic("not implemented") }
func (h destructBloomHasher) Size() int                         { return 8 }
func (h destructBloomHasher) Sum64() uint64 {
	return binary.BigEndian.Uint64(h[bloomDestructHasherOffset : bloomDestructHasherOffset+8])
}




type accountBloomHasher common.Hash

func (h accountBloomHasher) Write(p []byte) (n int, err error) { panic("not implemented") }
func (h accountBloomHasher) Sum(b []byte) []byte               { panic("not implemented") }
func (h accountBloomHasher) Reset()                            { panic("not implemented") }
func (h accountBloomHasher) BlockSize() int                    { panic("not implemented") }
func (h accountBloomHasher) Size() int                         { return 8 }
func (h accountBloomHasher) Sum64() uint64 {
	return binary.BigEndian.Uint64(h[bloomAccountHasherOffset : bloomAccountHasherOffset+8])
}




type storageBloomHasher [2]common.Hash

func (h storageBloomHasher) Write(p []byte) (n int, err error) { panic("not implemented") }
func (h storageBloomHasher) Sum(b []byte) []byte               { panic("not implemented") }
func (h storageBloomHasher) Reset()                            { panic("not implemented") }
func (h storageBloomHasher) BlockSize() int                    { panic("not implemented") }
func (h storageBloomHasher) Size() int                         { return 8 }
func (h storageBloomHasher) Sum64() uint64 {
	return binary.BigEndian.Uint64(h[0][bloomStorageHasherOffset:bloomStorageHasherOffset+8]) ^
		binary.BigEndian.Uint64(h[1][bloomStorageHasherOffset:bloomStorageHasherOffset+8])
}



func newDiffLayer(parent snapshot, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storage map[common.Hash]map[common.Hash][]byte) *diffLayer {
	
	dl := &diffLayer{
		parent:      parent,
		root:        root,
		destructSet: destructs,
		accountData: accounts,
		storageData: storage,
		storageList: make(map[common.Hash][]common.Hash),
	}
	switch parent := parent.(type) {
	case *diskLayer:
		dl.rebloom(parent)
	case *diffLayer:
		dl.rebloom(parent.origin)
	default:
		panic("unknown parent type")
	}
	
	for accountHash, blob := range accounts {
		if blob == nil {
			panic(fmt.Sprintf("account %#x nil", accountHash))
		}
	}
	for accountHash, slots := range storage {
		if slots == nil {
			panic(fmt.Sprintf("storage %#x nil", accountHash))
		}
	}
	
	for _, data := range accounts {
		dl.memory += uint64(common.HashLength + len(data))
		snapshotDirtyAccountWriteMeter.Mark(int64(len(data)))
	}
	
	for _, slots := range storage {
		for _, data := range slots {
			dl.memory += uint64(common.HashLength + len(data))
			snapshotDirtyStorageWriteMeter.Mark(int64(len(data)))
		}
	}
	dl.memory += uint64(len(destructs) * common.HashLength)
	return dl
}



func (dl *diffLayer) rebloom(origin *diskLayer) {
	dl.lock.Lock()
	defer dl.lock.Unlock()

	defer func(start time.Time) {
		snapshotBloomIndexTimer.Update(time.Since(start))
	}(time.Now())

	
	dl.origin = origin

	
	if parent, ok := dl.parent.(*diffLayer); ok {
		parent.lock.RLock()
		dl.diffed, _ = parent.diffed.Copy()
		parent.lock.RUnlock()
	} else {
		dl.diffed, _ = bloomfilter.New(uint64(bloomSize), uint64(bloomFuncs))
	}
	
	for hash := range dl.destructSet {
		dl.diffed.Add(destructBloomHasher(hash))
	}
	for hash := range dl.accountData {
		dl.diffed.Add(accountBloomHasher(hash))
	}
	for accountHash, slots := range dl.storageData {
		for storageHash := range slots {
			dl.diffed.Add(storageBloomHasher{accountHash, storageHash})
		}
	}
	
	
	
	k := float64(dl.diffed.K())
	n := float64(dl.diffed.N())
	m := float64(dl.diffed.M())
	snapshotBloomErrorGauge.Update(math.Pow(1.0-math.Exp((-k)*(n+0.5)/(m-1)), k))
}


func (dl *diffLayer) Root() common.Hash {
	return dl.root
}


func (dl *diffLayer) Parent() snapshot {
	return dl.parent
}



func (dl *diffLayer) Stale() bool {
	return atomic.LoadUint32(&dl.stale) != 0
}



func (dl *diffLayer) Account(hash common.Hash) (*Account, error) {
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





func (dl *diffLayer) AccountRLP(hash common.Hash) ([]byte, error) {
	
	
	dl.lock.RLock()
	hit := dl.diffed.Contains(accountBloomHasher(hash))
	if !hit {
		hit = dl.diffed.Contains(destructBloomHasher(hash))
	}
	dl.lock.RUnlock()

	
	
	if !hit {
		snapshotBloomAccountMissMeter.Mark(1)
		return dl.origin.AccountRLP(hash)
	}
	
	return dl.accountRLP(hash, 0)
}




func (dl *diffLayer) accountRLP(hash common.Hash, depth int) ([]byte, error) {
	dl.lock.RLock()
	defer dl.lock.RUnlock()

	
	
	if dl.Stale() {
		return nil, ErrSnapshotStale
	}
	
	if data, ok := dl.accountData[hash]; ok {
		snapshotDirtyAccountHitMeter.Mark(1)
		snapshotDirtyAccountHitDepthHist.Update(int64(depth))
		snapshotDirtyAccountReadMeter.Mark(int64(len(data)))
		snapshotBloomAccountTrueHitMeter.Mark(1)
		return data, nil
	}
	
	if _, ok := dl.destructSet[hash]; ok {
		snapshotDirtyAccountHitMeter.Mark(1)
		snapshotDirtyAccountHitDepthHist.Update(int64(depth))
		snapshotDirtyAccountInexMeter.Mark(1)
		snapshotBloomAccountTrueHitMeter.Mark(1)
		return nil, nil
	}
	
	if diff, ok := dl.parent.(*diffLayer); ok {
		return diff.accountRLP(hash, depth+1)
	}
	
	snapshotBloomAccountFalseHitMeter.Mark(1)
	return dl.parent.AccountRLP(hash)
}






func (dl *diffLayer) Storage(accountHash, storageHash common.Hash) ([]byte, error) {
	
	
	dl.lock.RLock()
	hit := dl.diffed.Contains(storageBloomHasher{accountHash, storageHash})
	if !hit {
		hit = dl.diffed.Contains(destructBloomHasher(accountHash))
	}
	dl.lock.RUnlock()

	
	
	if !hit {
		snapshotBloomStorageMissMeter.Mark(1)
		return dl.origin.Storage(accountHash, storageHash)
	}
	
	return dl.storage(accountHash, storageHash, 0)
}




func (dl *diffLayer) storage(accountHash, storageHash common.Hash, depth int) ([]byte, error) {
	dl.lock.RLock()
	defer dl.lock.RUnlock()

	
	
	if dl.Stale() {
		return nil, ErrSnapshotStale
	}
	
	if storage, ok := dl.storageData[accountHash]; ok {
		if data, ok := storage[storageHash]; ok {
			snapshotDirtyStorageHitMeter.Mark(1)
			snapshotDirtyStorageHitDepthHist.Update(int64(depth))
			if n := len(data); n > 0 {
				snapshotDirtyStorageReadMeter.Mark(int64(n))
			} else {
				snapshotDirtyStorageInexMeter.Mark(1)
			}
			snapshotBloomStorageTrueHitMeter.Mark(1)
			return data, nil
		}
	}
	
	if _, ok := dl.destructSet[accountHash]; ok {
		snapshotDirtyStorageHitMeter.Mark(1)
		snapshotDirtyStorageHitDepthHist.Update(int64(depth))
		snapshotDirtyStorageInexMeter.Mark(1)
		snapshotBloomStorageTrueHitMeter.Mark(1)
		return nil, nil
	}
	
	if diff, ok := dl.parent.(*diffLayer); ok {
		return diff.storage(accountHash, storageHash, depth+1)
	}
	
	snapshotBloomStorageFalseHitMeter.Mark(1)
	return dl.parent.Storage(accountHash, storageHash)
}



func (dl *diffLayer) Update(blockRoot common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storage map[common.Hash]map[common.Hash][]byte) *diffLayer {
	return newDiffLayer(dl, blockRoot, destructs, accounts, storage)
}




func (dl *diffLayer) flatten() snapshot {
	
	parent, ok := dl.parent.(*diffLayer)
	if !ok {
		return dl
	}
	
	
	
	parent = parent.flatten().(*diffLayer)

	parent.lock.Lock()
	defer parent.lock.Unlock()

	
	
	if atomic.SwapUint32(&parent.stale, 1) != 0 {
		panic("parent diff layer is stale") 
	}
	
	for hash := range dl.destructSet {
		parent.destructSet[hash] = struct{}{}
		delete(parent.accountData, hash)
		delete(parent.storageData, hash)
	}
	for hash, data := range dl.accountData {
		parent.accountData[hash] = data
	}
	
	for accountHash, storage := range dl.storageData {
		
		if _, ok := parent.storageData[accountHash]; !ok {
			parent.storageData[accountHash] = storage
			continue
		}
		
		comboData := parent.storageData[accountHash]
		for storageHash, data := range storage {
			comboData[storageHash] = data
		}
		parent.storageData[accountHash] = comboData
	}
	
	return &diffLayer{
		parent:      parent.parent,
		origin:      parent.origin,
		root:        dl.root,
		destructSet: parent.destructSet,
		accountData: parent.accountData,
		storageData: parent.storageData,
		storageList: make(map[common.Hash][]common.Hash),
		diffed:      dl.diffed,
		memory:      parent.memory + dl.memory,
	}
}





func (dl *diffLayer) AccountList() []common.Hash {
	
	dl.lock.RLock()
	list := dl.accountList
	dl.lock.RUnlock()

	if list != nil {
		return list
	}
	
	dl.lock.Lock()
	defer dl.lock.Unlock()

	dl.accountList = make([]common.Hash, 0, len(dl.destructSet)+len(dl.accountData))
	for hash := range dl.accountData {
		dl.accountList = append(dl.accountList, hash)
	}
	for hash := range dl.destructSet {
		if _, ok := dl.accountData[hash]; !ok {
			dl.accountList = append(dl.accountList, hash)
		}
	}
	sort.Sort(hashes(dl.accountList))
	dl.memory += uint64(len(dl.accountList) * common.HashLength)
	return dl.accountList
}










func (dl *diffLayer) StorageList(accountHash common.Hash) ([]common.Hash, bool) {
	dl.lock.RLock()
	_, destructed := dl.destructSet[accountHash]
	if _, ok := dl.storageData[accountHash]; !ok {
		
		dl.lock.RUnlock()
		return nil, destructed
	}
	
	if list, exist := dl.storageList[accountHash]; exist {
		dl.lock.RUnlock()
		return list, destructed 
	}
	dl.lock.RUnlock()

	
	dl.lock.Lock()
	defer dl.lock.Unlock()

	storageMap := dl.storageData[accountHash]
	storageList := make([]common.Hash, 0, len(storageMap))
	for k := range storageMap {
		storageList = append(storageList, k)
	}
	sort.Sort(hashes(storageList))
	dl.storageList[accountHash] = storageList
	dl.memory += uint64(len(dl.storageList)*common.HashLength + common.HashLength)
	return storageList, destructed
}
