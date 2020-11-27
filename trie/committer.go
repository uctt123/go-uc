















package trie

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
)



const leafChanSize = 200


type leaf struct {
	size int         
	hash common.Hash 
	node node        
}







type committer struct {
	tmp sliceBuffer
	sha crypto.KeccakState

	onleaf LeafCallback
	leafCh chan *leaf
}


var committerPool = sync.Pool{
	New: func() interface{} {
		return &committer{
			tmp: make(sliceBuffer, 0, 550), 
			sha: sha3.NewLegacyKeccak256().(crypto.KeccakState),
		}
	},
}


func newCommitter() *committer {
	return committerPool.Get().(*committer)
}

func returnCommitterToPool(h *committer) {
	h.onleaf = nil
	h.leafCh = nil
	committerPool.Put(h)
}


func (c *committer) Commit(n node, db *Database) (hashNode, error) {
	if db == nil {
		return nil, errors.New("no db provided")
	}
	h, err := c.commit(n, db)
	if err != nil {
		return nil, err
	}
	return h.(hashNode), nil
}


func (c *committer) commit(n node, db *Database) (node, error) {
	
	hash, dirty := n.cache()
	if hash != nil && !dirty {
		return hash, nil
	}
	
	switch cn := n.(type) {
	case *shortNode:
		
		collapsed := cn.copy()

		
		
		if _, ok := cn.Val.(*fullNode); ok {
			childV, err := c.commit(cn.Val, db)
			if err != nil {
				return nil, err
			}
			collapsed.Val = childV
		}
		
		collapsed.Key = hexToCompact(cn.Key)
		hashedNode := c.store(collapsed, db)
		if hn, ok := hashedNode.(hashNode); ok {
			return hn, nil
		}
		return collapsed, nil
	case *fullNode:
		hashedKids, err := c.commitChildren(cn, db)
		if err != nil {
			return nil, err
		}
		collapsed := cn.copy()
		collapsed.Children = hashedKids

		hashedNode := c.store(collapsed, db)
		if hn, ok := hashedNode.(hashNode); ok {
			return hn, nil
		}
		return collapsed, nil
	case hashNode:
		return cn, nil
	default:
		
		panic(fmt.Sprintf("%T: invalid node: %v", n, n))
	}
}


func (c *committer) commitChildren(n *fullNode, db *Database) ([17]node, error) {
	var children [17]node
	for i := 0; i < 16; i++ {
		child := n.Children[i]
		if child == nil {
			continue
		}
		
		
		
		if hn, ok := child.(hashNode); ok {
			children[i] = hn
			continue
		}
		
		
		
		hashed, err := c.commit(child, db)
		if err != nil {
			return children, err
		}
		children[i] = hashed
	}
	
	if n.Children[16] != nil {
		children[16] = n.Children[16]
	}
	return children, nil
}




func (c *committer) store(n node, db *Database) node {
	
	var (
		hash, _ = n.cache()
		size    int
	)
	if hash == nil {
		
		
		
		
		return n
	} else {
		
		
		size = estimateSize(n)
	}
	
	
	if c.leafCh != nil {
		c.leafCh <- &leaf{
			size: size,
			hash: common.BytesToHash(hash),
			node: n,
		}
	} else if db != nil {
		
		
		db.lock.Lock()
		db.insert(common.BytesToHash(hash), size, n)
		db.lock.Unlock()
	}
	return hash
}


func (c *committer) commitLoop(db *Database) {
	for item := range c.leafCh {
		var (
			hash = item.hash
			size = item.size
			n    = item.node
		)
		
		db.lock.Lock()
		db.insert(hash, size, n)
		db.lock.Unlock()

		if c.onleaf != nil {
			switch n := n.(type) {
			case *shortNode:
				if child, ok := n.Val.(valueNode); ok {
					c.onleaf(nil, child, hash)
				}
			case *fullNode:
				
				
				if n.Children[16] != nil {
					c.onleaf(nil, n.Children[16].(valueNode), hash)
				}
			}
		}
	}
}

func (c *committer) makeHashNode(data []byte) hashNode {
	n := make(hashNode, c.sha.Size())
	c.sha.Reset()
	c.sha.Write(data)
	c.sha.Read(n)
	return n
}





func estimateSize(n node) int {
	switch n := n.(type) {
	case *shortNode:
		
		return 3 + len(n.Key) + estimateSize(n.Val)
	case *fullNode:
		
		s := 3
		for i := 0; i < 16; i++ {
			if child := n.Children[i]; child != nil {
				s += estimateSize(child)
			} else {
				s++
			}
		}
		return s
	case valueNode:
		return 1 + len(n)
	case hashNode:
		return 1 + len(n)
	default:
		panic(fmt.Sprintf("node type %T", n))

	}
}
