















package snapshot

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
)




type weightedIterator struct {
	it       Iterator
	priority int
}


type weightedIterators []*weightedIterator


func (its weightedIterators) Len() int { return len(its) }



func (its weightedIterators) Less(i, j int) bool {
	
	hashI := its[i].it.Hash()
	hashJ := its[j].it.Hash()

	switch bytes.Compare(hashI[:], hashJ[:]) {
	case -1:
		return true
	case 1:
		return false
	}
	
	return its[i].priority < its[j].priority
}


func (its weightedIterators) Swap(i, j int) {
	its[i], its[j] = its[j], its[i]
}



type fastIterator struct {
	tree *Tree       
	root common.Hash 

	curAccount []byte
	curSlot    []byte

	iterators weightedIterators
	initiated bool
	account   bool
	fail      error
}




func newFastIterator(tree *Tree, root common.Hash, account common.Hash, seek common.Hash, accountIterator bool) (*fastIterator, error) {
	snap := tree.Snapshot(root)
	if snap == nil {
		return nil, fmt.Errorf("unknown snapshot: %x", root)
	}
	fi := &fastIterator{
		tree:    tree,
		root:    root,
		account: accountIterator,
	}
	current := snap.(snapshot)
	for depth := 0; current != nil; depth++ {
		if accountIterator {
			fi.iterators = append(fi.iterators, &weightedIterator{
				it:       current.AccountIterator(seek),
				priority: depth,
			})
		} else {
			
			
			
			
			it, destructed := current.StorageIterator(account, seek)
			fi.iterators = append(fi.iterators, &weightedIterator{
				it:       it,
				priority: depth,
			})
			if destructed {
				break
			}
		}
		current = current.Parent()
	}
	fi.init()
	return fi, nil
}



func (fi *fastIterator) init() {
	
	var positioned = make(map[common.Hash]int)

	
	for i := 0; i < len(fi.iterators); i++ {
		
		
		
		it := fi.iterators[i]
		for {
			
			if !it.it.Next() {
				it.it.Release()
				last := len(fi.iterators) - 1

				fi.iterators[i] = fi.iterators[last]
				fi.iterators[last] = nil
				fi.iterators = fi.iterators[:last]

				i--
				break
			}
			
			hash := it.it.Hash()
			if other, exist := positioned[hash]; !exist {
				positioned[hash] = i
				break
			} else {
				
				
				
				
				
				
				
				
				if fi.iterators[other].priority < it.priority {
					
					continue
				} else {
					
					it = fi.iterators[other]
					fi.iterators[other], fi.iterators[i] = fi.iterators[i], fi.iterators[other]
					continue
				}
			}
		}
	}
	
	sort.Sort(fi.iterators)
	fi.initiated = false
}


func (fi *fastIterator) Next() bool {
	if len(fi.iterators) == 0 {
		return false
	}
	if !fi.initiated {
		
		
		fi.initiated = true
		if fi.account {
			fi.curAccount = fi.iterators[0].it.(AccountIterator).Account()
		} else {
			fi.curSlot = fi.iterators[0].it.(StorageIterator).Slot()
		}
		if innerErr := fi.iterators[0].it.Error(); innerErr != nil {
			fi.fail = innerErr
			return false
		}
		if fi.curAccount != nil || fi.curSlot != nil {
			return true
		}
		
		
	}
	
	
	
	
	
	
	
	for {
		if !fi.next(0) {
			return false 
		}
		if fi.account {
			fi.curAccount = fi.iterators[0].it.(AccountIterator).Account()
		} else {
			fi.curSlot = fi.iterators[0].it.(StorageIterator).Slot()
		}
		if innerErr := fi.iterators[0].it.Error(); innerErr != nil {
			fi.fail = innerErr
			return false 
		}
		if fi.curAccount != nil || fi.curSlot != nil {
			break 
		}
	}
	return true
}







func (fi *fastIterator) next(idx int) bool {
	
	
	
	if it := fi.iterators[idx].it; !it.Next() {
		it.Release()

		fi.iterators = append(fi.iterators[:idx], fi.iterators[idx+1:]...)
		return len(fi.iterators) > 0
	}
	
	if idx == len(fi.iterators)-1 {
		return true
	}
	
	var (
		cur, next         = fi.iterators[idx], fi.iterators[idx+1]
		curHash, nextHash = cur.it.Hash(), next.it.Hash()
	)
	if diff := bytes.Compare(curHash[:], nextHash[:]); diff < 0 {
		
		return true
	} else if diff == 0 && cur.priority < next.priority {
		
		fi.next(idx + 1)
		return true
	}
	
	
	clash := -1
	index := sort.Search(len(fi.iterators), func(n int) bool {
		
		
		
		if n < idx {
			return false
		}
		if n == len(fi.iterators)-1 {
			
			return true
		}
		nextHash := fi.iterators[n+1].it.Hash()
		if diff := bytes.Compare(curHash[:], nextHash[:]); diff < 0 {
			return true
		} else if diff > 0 {
			return false
		}
		
		
		clash = n + 1

		return cur.priority < fi.iterators[n+1].priority
	})
	fi.move(idx, index)
	if clash != -1 {
		fi.next(clash)
	}
	return true
}


func (fi *fastIterator) move(index, newpos int) {
	elem := fi.iterators[index]
	copy(fi.iterators[index:], fi.iterators[index+1:newpos+1])
	fi.iterators[newpos] = elem
}



func (fi *fastIterator) Error() error {
	return fi.fail
}


func (fi *fastIterator) Hash() common.Hash {
	return fi.iterators[0].it.Hash()
}



func (fi *fastIterator) Account() []byte {
	return fi.curAccount
}



func (fi *fastIterator) Slot() []byte {
	return fi.curSlot
}



func (fi *fastIterator) Release() {
	for _, it := range fi.iterators {
		it.it.Release()
	}
	fi.iterators = nil
}


func (fi *fastIterator) Debug() {
	for _, it := range fi.iterators {
		fmt.Printf("[p=%v v=%v] ", it.priority, it.it.Hash()[0])
	}
	fmt.Println()
}




func newFastAccountIterator(tree *Tree, root common.Hash, seek common.Hash) (AccountIterator, error) {
	return newFastIterator(tree, root, common.Hash{}, seek, true)
}




func newFastStorageIterator(tree *Tree, root common.Hash, account common.Hash, seek common.Hash) (StorageIterator, error) {
	return newFastIterator(tree, root, account, seek, false)
}
