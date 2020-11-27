















package snapshot

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
)




type binaryIterator struct {
	a               Iterator
	b               Iterator
	aDone           bool
	bDone           bool
	accountIterator bool
	k               common.Hash
	account         common.Hash
	fail            error
}




func (dl *diffLayer) initBinaryAccountIterator() Iterator {
	parent, ok := dl.parent.(*diffLayer)
	if !ok {
		l := &binaryIterator{
			a:               dl.AccountIterator(common.Hash{}),
			b:               dl.Parent().AccountIterator(common.Hash{}),
			accountIterator: true,
		}
		l.aDone = !l.a.Next()
		l.bDone = !l.b.Next()
		return l
	}
	l := &binaryIterator{
		a:               dl.AccountIterator(common.Hash{}),
		b:               parent.initBinaryAccountIterator(),
		accountIterator: true,
	}
	l.aDone = !l.a.Next()
	l.bDone = !l.b.Next()
	return l
}




func (dl *diffLayer) initBinaryStorageIterator(account common.Hash) Iterator {
	parent, ok := dl.parent.(*diffLayer)
	if !ok {
		
		
		a, destructed := dl.StorageIterator(account, common.Hash{})
		if destructed {
			l := &binaryIterator{
				a:       a,
				account: account,
			}
			l.aDone = !l.a.Next()
			l.bDone = true
			return l
		}
		
		
		b, _ := dl.Parent().StorageIterator(account, common.Hash{})
		l := &binaryIterator{
			a:       a,
			b:       b,
			account: account,
		}
		l.aDone = !l.a.Next()
		l.bDone = !l.b.Next()
		return l
	}
	
	
	a, destructed := dl.StorageIterator(account, common.Hash{})
	if destructed {
		l := &binaryIterator{
			a:       a,
			account: account,
		}
		l.aDone = !l.a.Next()
		l.bDone = true
		return l
	}
	l := &binaryIterator{
		a:       a,
		b:       parent.initBinaryStorageIterator(account),
		account: account,
	}
	l.aDone = !l.a.Next()
	l.bDone = !l.b.Next()
	return l
}




func (it *binaryIterator) Next() bool {
	if it.aDone && it.bDone {
		return false
	}
first:
	if it.aDone {
		it.k = it.b.Hash()
		it.bDone = !it.b.Next()
		return true
	}
	if it.bDone {
		it.k = it.a.Hash()
		it.aDone = !it.a.Next()
		return true
	}
	nextA, nextB := it.a.Hash(), it.b.Hash()
	if diff := bytes.Compare(nextA[:], nextB[:]); diff < 0 {
		it.aDone = !it.a.Next()
		it.k = nextA
		return true
	} else if diff == 0 {
		
		it.aDone = !it.a.Next()
		goto first
	}
	it.bDone = !it.b.Next()
	it.k = nextB
	return true
}



func (it *binaryIterator) Error() error {
	return it.fail
}


func (it *binaryIterator) Hash() common.Hash {
	return it.k
}






func (it *binaryIterator) Account() []byte {
	if !it.accountIterator {
		return nil
	}
	
	blob, err := it.a.(*diffAccountIterator).layer.AccountRLP(it.k)
	if err != nil {
		it.fail = err
		return nil
	}
	return blob
}






func (it *binaryIterator) Slot() []byte {
	if it.accountIterator {
		return nil
	}
	blob, err := it.a.(*diffStorageIterator).layer.Storage(it.account, it.k)
	if err != nil {
		it.fail = err
		return nil
	}
	return blob
}


func (it *binaryIterator) Release() {
	it.a.Release()
	it.b.Release()
}



func (dl *diffLayer) newBinaryAccountIterator() AccountIterator {
	iter := dl.initBinaryAccountIterator()
	return iter.(AccountIterator)
}



func (dl *diffLayer) newBinaryStorageIterator(account common.Hash) StorageIterator {
	iter := dl.initBinaryStorageIterator(account)
	return iter.(StorageIterator)
}
