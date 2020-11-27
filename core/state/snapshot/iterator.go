















package snapshot

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
)



type Iterator interface {
	
	
	
	Next() bool

	
	
	Error() error

	
	
	Hash() common.Hash

	
	
	Release()
}



type AccountIterator interface {
	Iterator

	
	
	Account() []byte
}



type StorageIterator interface {
	Iterator

	
	
	Slot() []byte
}




type diffAccountIterator struct {
	
	
	
	
	curHash common.Hash

	layer *diffLayer    
	keys  []common.Hash 
	fail  error         
}


func (dl *diffLayer) AccountIterator(seek common.Hash) AccountIterator {
	
	hashes := dl.AccountList()
	index := sort.Search(len(hashes), func(i int) bool {
		return bytes.Compare(seek[:], hashes[i][:]) <= 0
	})
	
	return &diffAccountIterator{
		layer: dl,
		keys:  hashes[index:],
	}
}


func (it *diffAccountIterator) Next() bool {
	
	
	
	
	if it.fail != nil {
		panic(fmt.Sprintf("called Next of failed iterator: %v", it.fail))
	}
	
	if len(it.keys) == 0 {
		return false
	}
	if it.layer.Stale() {
		it.fail, it.keys = ErrSnapshotStale, nil
		return false
	}
	
	it.curHash = it.keys[0]
	
	it.keys = it.keys[1:]
	return true
}



func (it *diffAccountIterator) Error() error {
	return it.fail
}


func (it *diffAccountIterator) Hash() common.Hash {
	return it.curHash
}









func (it *diffAccountIterator) Account() []byte {
	it.layer.lock.RLock()
	blob, ok := it.layer.accountData[it.curHash]
	if !ok {
		if _, ok := it.layer.destructSet[it.curHash]; ok {
			it.layer.lock.RUnlock()
			return nil
		}
		panic(fmt.Sprintf("iterator referenced non-existent account: %x", it.curHash))
	}
	it.layer.lock.RUnlock()
	if it.layer.Stale() {
		it.fail, it.keys = ErrSnapshotStale, nil
	}
	return blob
}


func (it *diffAccountIterator) Release() {}



type diskAccountIterator struct {
	layer *diskLayer
	it    ethdb.Iterator
}


func (dl *diskLayer) AccountIterator(seek common.Hash) AccountIterator {
	pos := common.TrimRightZeroes(seek[:])
	return &diskAccountIterator{
		layer: dl,
		it:    dl.diskdb.NewIterator(rawdb.SnapshotAccountPrefix, pos),
	}
}


func (it *diskAccountIterator) Next() bool {
	
	if it.it == nil {
		return false
	}
	
	for {
		if !it.it.Next() {
			it.it.Release()
			it.it = nil
			return false
		}
		if len(it.it.Key()) == len(rawdb.SnapshotAccountPrefix)+common.HashLength {
			break
		}
	}
	return true
}






func (it *diskAccountIterator) Error() error {
	if it.it == nil {
		return nil 
	}
	return it.it.Error()
}


func (it *diskAccountIterator) Hash() common.Hash {
	return common.BytesToHash(it.it.Key()) 
}


func (it *diskAccountIterator) Account() []byte {
	return it.it.Value()
}


func (it *diskAccountIterator) Release() {
	
	if it.it != nil {
		it.it.Release()
		it.it = nil
	}
}




type diffStorageIterator struct {
	
	
	
	
	curHash common.Hash
	account common.Hash

	layer *diffLayer    
	keys  []common.Hash 
	fail  error         
}






func (dl *diffLayer) StorageIterator(account common.Hash, seek common.Hash) (StorageIterator, bool) {
	
	
	
	hashes, destructed := dl.StorageList(account)
	index := sort.Search(len(hashes), func(i int) bool {
		return bytes.Compare(seek[:], hashes[i][:]) <= 0
	})
	
	return &diffStorageIterator{
		layer:   dl,
		account: account,
		keys:    hashes[index:],
	}, destructed
}


func (it *diffStorageIterator) Next() bool {
	
	
	
	
	if it.fail != nil {
		panic(fmt.Sprintf("called Next of failed iterator: %v", it.fail))
	}
	
	if len(it.keys) == 0 {
		return false
	}
	if it.layer.Stale() {
		it.fail, it.keys = ErrSnapshotStale, nil
		return false
	}
	
	it.curHash = it.keys[0]
	
	it.keys = it.keys[1:]
	return true
}



func (it *diffStorageIterator) Error() error {
	return it.fail
}


func (it *diffStorageIterator) Hash() common.Hash {
	return it.curHash
}









func (it *diffStorageIterator) Slot() []byte {
	it.layer.lock.RLock()
	storage, ok := it.layer.storageData[it.account]
	if !ok {
		panic(fmt.Sprintf("iterator referenced non-existent account storage: %x", it.account))
	}
	
	blob, ok := storage[it.curHash]
	if !ok {
		panic(fmt.Sprintf("iterator referenced non-existent storage slot: %x", it.curHash))
	}
	it.layer.lock.RUnlock()
	if it.layer.Stale() {
		it.fail, it.keys = ErrSnapshotStale, nil
	}
	return blob
}


func (it *diffStorageIterator) Release() {}



type diskStorageIterator struct {
	layer   *diskLayer
	account common.Hash
	it      ethdb.Iterator
}





func (dl *diskLayer) StorageIterator(account common.Hash, seek common.Hash) (StorageIterator, bool) {
	pos := common.TrimRightZeroes(seek[:])
	return &diskStorageIterator{
		layer:   dl,
		account: account,
		it:      dl.diskdb.NewIterator(append(rawdb.SnapshotStoragePrefix, account.Bytes()...), pos),
	}, false
}


func (it *diskStorageIterator) Next() bool {
	
	if it.it == nil {
		return false
	}
	
	for {
		if !it.it.Next() {
			it.it.Release()
			it.it = nil
			return false
		}
		if len(it.it.Key()) == len(rawdb.SnapshotStoragePrefix)+common.HashLength+common.HashLength {
			break
		}
	}
	return true
}






func (it *diskStorageIterator) Error() error {
	if it.it == nil {
		return nil 
	}
	return it.it.Error()
}


func (it *diskStorageIterator) Hash() common.Hash {
	return common.BytesToHash(it.it.Key()) 
}


func (it *diskStorageIterator) Slot() []byte {
	return it.it.Value()
}


func (it *diskStorageIterator) Release() {
	
	if it.it != nil {
		it.it.Release()
		it.it = nil
	}
}
