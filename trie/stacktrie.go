















package trie

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrCommitDisabled = errors.New("no database for committing")

var stPool = sync.Pool{
	New: func() interface{} {
		return NewStackTrie(nil)
	},
}

func stackTrieFromPool(db ethdb.KeyValueStore) *StackTrie {
	st := stPool.Get().(*StackTrie)
	st.db = db
	return st
}

func returnToPool(st *StackTrie) {
	st.Reset()
	stPool.Put(st)
}




type StackTrie struct {
	nodeType  uint8          
	val       []byte         
	key       []byte         
	keyOffset int            
	children  [16]*StackTrie 

	db ethdb.KeyValueStore 
}


func NewStackTrie(db ethdb.KeyValueStore) *StackTrie {
	return &StackTrie{
		nodeType: emptyNode,
		db:       db,
	}
}

func newLeaf(ko int, key, val []byte, db ethdb.KeyValueStore) *StackTrie {
	st := stackTrieFromPool(db)
	st.nodeType = leafNode
	st.keyOffset = ko
	st.key = append(st.key, key[ko:]...)
	st.val = val
	return st
}

func newExt(ko int, key []byte, child *StackTrie, db ethdb.KeyValueStore) *StackTrie {
	st := stackTrieFromPool(db)
	st.nodeType = extNode
	st.keyOffset = ko
	st.key = append(st.key, key[ko:]...)
	st.children[0] = child
	return st
}


const (
	emptyNode = iota
	branchNode
	extNode
	leafNode
	hashedNode
)


func (st *StackTrie) TryUpdate(key, value []byte) error {
	k := keybytesToHex(key)
	if len(value) == 0 {
		panic("deletion not supported")
	}
	st.insert(k[:len(k)-1], value)
	return nil
}

func (st *StackTrie) Update(key, value []byte) {
	if err := st.TryUpdate(key, value); err != nil {
		log.Error(fmt.Sprintf("Unhandled trie error: %v", err))
	}
}

func (st *StackTrie) Reset() {
	st.db = nil
	st.key = st.key[:0]
	st.val = nil
	for i := range st.children {
		st.children[i] = nil
	}
	st.nodeType = emptyNode
	st.keyOffset = 0
}




func (st *StackTrie) getDiffIndex(key []byte) int {
	diffindex := 0
	for ; diffindex < len(st.key) && st.key[diffindex] == key[st.keyOffset+diffindex]; diffindex++ {
	}
	return diffindex
}



func (st *StackTrie) insert(key, value []byte) {
	switch st.nodeType {
	case branchNode: /* Branch */
		idx := int(key[st.keyOffset])
		
		for i := idx - 1; i >= 0; i-- {
			if st.children[i] != nil {
				if st.children[i].nodeType != hashedNode {
					st.children[i].hash()
				}
				break
			}
		}
		
		if st.children[idx] == nil {
			st.children[idx] = stackTrieFromPool(st.db)
			st.children[idx].keyOffset = st.keyOffset + 1
		}
		st.children[idx].insert(key, value)
	case extNode: /* Ext */
		
		diffidx := st.getDiffIndex(key)

		
		
		
		
		
		if diffidx == len(st.key) {
			
			
			st.children[0].insert(key, value)
			return
		}
		
		
		
		
		var n *StackTrie
		if diffidx < len(st.key)-1 {
			n = newExt(diffidx+1, st.key, st.children[0], st.db)
		} else {
			
			
			n = st.children[0]
		}
		
		n.hash()
		var p *StackTrie
		if diffidx == 0 {
			
			
			
			st.children[0] = nil
			p = st
			st.nodeType = branchNode
		} else {
			
			
			
			st.children[0] = stackTrieFromPool(st.db)
			st.children[0].nodeType = branchNode
			st.children[0].keyOffset = st.keyOffset + diffidx
			p = st.children[0]
		}
		
		o := newLeaf(st.keyOffset+diffidx+1, key, value, st.db)

		
		origIdx := st.key[diffidx]
		newIdx := key[diffidx+st.keyOffset]
		p.children[origIdx] = n
		p.children[newIdx] = o
		st.key = st.key[:diffidx]

	case leafNode: /* Leaf */
		
		diffidx := st.getDiffIndex(key)

		
		
		
		
		
		
		if diffidx >= len(st.key) {
			panic("Trying to insert into existing key")
		}

		
		
		
		var p *StackTrie
		if diffidx == 0 {
			
			st.nodeType = branchNode
			p = st
			st.children[0] = nil
		} else {
			
			
			st.nodeType = extNode
			st.children[0] = NewStackTrie(st.db)
			st.children[0].nodeType = branchNode
			st.children[0].keyOffset = st.keyOffset + diffidx
			p = st.children[0]
		}

		
		
		
		
		origIdx := st.key[diffidx]
		p.children[origIdx] = newLeaf(diffidx+1, st.key, st.val, st.db)
		p.children[origIdx].hash()

		newIdx := key[diffidx+st.keyOffset]
		p.children[newIdx] = newLeaf(p.keyOffset+1, key, value, st.db)

		
		
		st.key = st.key[:diffidx]
		st.val = nil
	case emptyNode: /* Empty */
		st.nodeType = leafNode
		st.key = key[st.keyOffset:]
		st.val = value
	case hashedNode:
		panic("trying to insert into hash")
	default:
		panic("invalid type")
	}
}













func (st *StackTrie) hash() {
	/* Shortcut if node is already hashed */
	if st.nodeType == hashedNode {
		return
	}
	
	
	
	var h *hasher

	switch st.nodeType {
	case branchNode:
		var nodes [17]node
		for i, child := range st.children {
			if child == nil {
				nodes[i] = nilValueNode
				continue
			}
			child.hash()
			if len(child.val) < 32 {
				nodes[i] = rawNode(child.val)
			} else {
				nodes[i] = hashNode(child.val)
			}
			st.children[i] = nil 
			returnToPool(child)
		}
		nodes[16] = nilValueNode
		h = newHasher(false)
		defer returnHasherToPool(h)
		h.tmp.Reset()
		if err := rlp.Encode(&h.tmp, nodes); err != nil {
			panic(err)
		}
	case extNode:
		h = newHasher(false)
		defer returnHasherToPool(h)
		h.tmp.Reset()
		st.children[0].hash()
		
		
		
		
		
		
		n := [][]byte{
			hexToCompact(st.key),
			st.children[0].val,
		}
		if err := rlp.Encode(&h.tmp, n); err != nil {
			panic(err)
		}
		returnToPool(st.children[0])
		st.children[0] = nil 
	case leafNode:
		h = newHasher(false)
		defer returnHasherToPool(h)
		h.tmp.Reset()
		st.key = append(st.key, byte(16))
		sz := hexToCompactInPlace(st.key)
		n := [][]byte{st.key[:sz], st.val}
		if err := rlp.Encode(&h.tmp, n); err != nil {
			panic(err)
		}
	case emptyNode:
		st.val = st.val[:0]
		st.val = append(st.val, emptyRoot[:]...)
		st.key = st.key[:0]
		st.nodeType = hashedNode
		return
	default:
		panic("Invalid node type")
	}
	st.key = st.key[:0]
	st.nodeType = hashedNode
	if len(h.tmp) < 32 {
		st.val = st.val[:0]
		st.val = append(st.val, h.tmp...)
		return
	}
	
	
	if required := 32 - len(st.val); required > 0 {
		buf := make([]byte, required)
		st.val = append(st.val, buf...)
	}
	st.val = st.val[:32]
	h.sha.Reset()
	h.sha.Write(h.tmp)
	h.sha.Read(st.val)
	if st.db != nil {
		
		
		st.db.Put(st.val, h.tmp)
	}
}


func (st *StackTrie) Hash() (h common.Hash) {
	st.hash()
	if len(st.val) != 32 {
		
		
		
		ret := make([]byte, 32)
		h := newHasher(false)
		defer returnHasherToPool(h)
		h.sha.Reset()
		h.sha.Write(st.val)
		h.sha.Read(ret)
		return common.BytesToHash(ret)
	}
	return common.BytesToHash(st.val)
}








func (st *StackTrie) Commit() (common.Hash, error) {
	if st.db == nil {
		return common.Hash{}, ErrCommitDisabled
	}
	st.hash()
	h := common.BytesToHash(st.val)
	return h, nil
}
