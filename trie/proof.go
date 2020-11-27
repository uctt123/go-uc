















package trie

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)








func (t *Trie) Prove(key []byte, fromLevel uint, proofDb ethdb.KeyValueWriter) error {
	
	key = keybytesToHex(key)
	var nodes []node
	tn := t.root
	for len(key) > 0 && tn != nil {
		switch n := tn.(type) {
		case *shortNode:
			if len(key) < len(n.Key) || !bytes.Equal(n.Key, key[:len(n.Key)]) {
				
				tn = nil
			} else {
				tn = n.Val
				key = key[len(n.Key):]
			}
			nodes = append(nodes, n)
		case *fullNode:
			tn = n.Children[key[0]]
			key = key[1:]
			nodes = append(nodes, n)
		case hashNode:
			var err error
			tn, err = t.resolveHash(n, nil)
			if err != nil {
				log.Error(fmt.Sprintf("Unhandled trie error: %v", err))
				return err
			}
		default:
			panic(fmt.Sprintf("%T: invalid node: %v", tn, tn))
		}
	}
	hasher := newHasher(false)
	defer returnHasherToPool(hasher)

	for i, n := range nodes {
		if fromLevel > 0 {
			fromLevel--
			continue
		}
		var hn node
		n, hn = hasher.proofHash(n)
		if hash, ok := hn.(hashNode); ok || i == 0 {
			
			
			enc, _ := rlp.EncodeToBytes(n)
			if !ok {
				hash = hasher.hashData(enc)
			}
			proofDb.Put(hash, enc)
		}
	}
	return nil
}








func (t *SecureTrie) Prove(key []byte, fromLevel uint, proofDb ethdb.KeyValueWriter) error {
	return t.trie.Prove(key, fromLevel, proofDb)
}




func VerifyProof(rootHash common.Hash, key []byte, proofDb ethdb.KeyValueReader) (value []byte, err error) {
	key = keybytesToHex(key)
	wantHash := rootHash
	for i := 0; ; i++ {
		buf, _ := proofDb.Get(wantHash[:])
		if buf == nil {
			return nil, fmt.Errorf("proof node %d (hash %064x) missing", i, wantHash)
		}
		n, err := decodeNode(wantHash[:], buf)
		if err != nil {
			return nil, fmt.Errorf("bad proof node %d: %v", i, err)
		}
		keyrest, cld := get(n, key, true)
		switch cld := cld.(type) {
		case nil:
			
			return nil, nil
		case hashNode:
			key = keyrest
			copy(wantHash[:], cld)
		case valueNode:
			return cld, nil
		}
	}
}






func proofToPath(rootHash common.Hash, root node, key []byte, proofDb ethdb.KeyValueReader, allowNonExistent bool) (node, []byte, error) {
	
	resolveNode := func(hash common.Hash) (node, error) {
		buf, _ := proofDb.Get(hash[:])
		if buf == nil {
			return nil, fmt.Errorf("proof node (hash %064x) missing", hash)
		}
		n, err := decodeNode(hash[:], buf)
		if err != nil {
			return nil, fmt.Errorf("bad proof node %v", err)
		}
		return n, err
	}
	
	
	if root == nil {
		n, err := resolveNode(rootHash)
		if err != nil {
			return nil, nil, err
		}
		root = n
	}
	var (
		err           error
		child, parent node
		keyrest       []byte
		valnode       []byte
	)
	key, parent = keybytesToHex(key), root
	for {
		keyrest, child = get(parent, key, false)
		switch cld := child.(type) {
		case nil:
			
			
			
			
			if allowNonExistent {
				return root, nil, nil
			}
			return nil, nil, errors.New("the node is not contained in trie")
		case *shortNode:
			key, parent = keyrest, child 
			continue
		case *fullNode:
			key, parent = keyrest, child 
			continue
		case hashNode:
			child, err = resolveNode(common.BytesToHash(cld))
			if err != nil {
				return nil, nil, err
			}
		case valueNode:
			valnode = cld
		}
		
		switch pnode := parent.(type) {
		case *shortNode:
			pnode.Val = child
		case *fullNode:
			pnode.Children[key[0]] = child
		default:
			panic(fmt.Sprintf("%T: invalid node: %v", pnode, pnode))
		}
		if len(valnode) > 0 {
			return root, valnode, nil 
		}
		key, parent = keyrest, child
	}
}












func unsetInternal(n node, left []byte, right []byte) error {
	left, right = keybytesToHex(left), keybytesToHex(right)

	
	
	
	
	
	var (
		pos    = 0
		parent node

		
		shortForkLeft, shortForkRight int
	)
findFork:
	for {
		switch rn := (n).(type) {
		case *shortNode:
			rn.flags = nodeFlag{dirty: true}

			
			
			if len(left)-pos < len(rn.Key) {
				shortForkLeft = bytes.Compare(left[pos:], rn.Key)
			} else {
				shortForkLeft = bytes.Compare(left[pos:pos+len(rn.Key)], rn.Key)
			}
			if len(right)-pos < len(rn.Key) {
				shortForkRight = bytes.Compare(right[pos:], rn.Key)
			} else {
				shortForkRight = bytes.Compare(right[pos:pos+len(rn.Key)], rn.Key)
			}
			if shortForkLeft != 0 || shortForkRight != 0 {
				break findFork
			}
			parent = n
			n, pos = rn.Val, pos+len(rn.Key)
		case *fullNode:
			rn.flags = nodeFlag{dirty: true}

			
			
			leftnode, rightnode := rn.Children[left[pos]], rn.Children[right[pos]]
			if leftnode == nil || rightnode == nil || leftnode != rightnode {
				break findFork
			}
			parent = n
			n, pos = rn.Children[left[pos]], pos+1
		default:
			panic(fmt.Sprintf("%T: invalid node: %v", n, n))
		}
	}
	switch rn := n.(type) {
	case *shortNode:
		
		
		
		
		
		
		if shortForkLeft == -1 && shortForkRight == -1 {
			return errors.New("empty range")
		}
		if shortForkLeft == 1 && shortForkRight == 1 {
			return errors.New("empty range")
		}
		if shortForkLeft != 0 && shortForkRight != 0 {
			parent.(*fullNode).Children[left[pos-1]] = nil
			return nil
		}
		
		if shortForkRight != 0 {
			
			if _, ok := rn.Val.(valueNode); ok {
				parent.(*fullNode).Children[left[pos-1]] = nil
				return nil
			}
			return unset(rn, rn.Val, left[pos:], len(rn.Key), false)
		}
		if shortForkLeft != 0 {
			
			if _, ok := rn.Val.(valueNode); ok {
				parent.(*fullNode).Children[right[pos-1]] = nil
				return nil
			}
			return unset(rn, rn.Val, right[pos:], len(rn.Key), true)
		}
		return nil
	case *fullNode:
		
		for i := left[pos] + 1; i < right[pos]; i++ {
			rn.Children[i] = nil
		}
		if err := unset(rn, rn.Children[left[pos]], left[pos:], 1, false); err != nil {
			return err
		}
		if err := unset(rn, rn.Children[right[pos]], right[pos:], 1, true); err != nil {
			return err
		}
		return nil
	default:
		panic(fmt.Sprintf("%T: invalid node: %v", n, n))
	}
}













func unset(parent node, child node, key []byte, pos int, removeLeft bool) error {
	switch cld := child.(type) {
	case *fullNode:
		if removeLeft {
			for i := 0; i < int(key[pos]); i++ {
				cld.Children[i] = nil
			}
			cld.flags = nodeFlag{dirty: true}
		} else {
			for i := key[pos] + 1; i < 16; i++ {
				cld.Children[i] = nil
			}
			cld.flags = nodeFlag{dirty: true}
		}
		return unset(cld, cld.Children[key[pos]], key, pos+1, removeLeft)
	case *shortNode:
		if len(key[pos:]) < len(cld.Key) || !bytes.Equal(cld.Key, key[pos:pos+len(cld.Key)]) {
			
			if removeLeft {
				if bytes.Compare(cld.Key, key[pos:]) < 0 {
					
					
					
					fn := parent.(*fullNode)
					fn.Children[key[pos-1]] = nil
				} else {
					
					
					
				}
			} else {
				if bytes.Compare(cld.Key, key[pos:]) > 0 {
					
					
					
					fn := parent.(*fullNode)
					fn.Children[key[pos-1]] = nil
				} else {
					
					
					
				}
			}
			return nil
		}
		if _, ok := cld.Val.(valueNode); ok {
			fn := parent.(*fullNode)
			fn.Children[key[pos-1]] = nil
			return nil
		}
		cld.flags = nodeFlag{dirty: true}
		return unset(cld, cld.Val, key, pos+len(cld.Key), removeLeft)
	case nil:
		
		
		return nil
	default:
		panic("it shouldn't happen") 
	}
}





func hasRightElement(node node, key []byte) bool {
	pos, key := 0, keybytesToHex(key)
	for node != nil {
		switch rn := node.(type) {
		case *fullNode:
			for i := key[pos] + 1; i < 16; i++ {
				if rn.Children[i] != nil {
					return true
				}
			}
			node, pos = rn.Children[key[pos]], pos+1
		case *shortNode:
			if len(key)-pos < len(rn.Key) || !bytes.Equal(rn.Key, key[pos:pos+len(rn.Key)]) {
				return bytes.Compare(rn.Key, key[pos:]) > 0
			}
			node, pos = rn.Val, pos+len(rn.Key)
		case valueNode:
			return false 
		default:
			panic(fmt.Sprintf("%T: invalid node: %v", node, node)) 
		}
	}
	return false
}































func VerifyRangeProof(rootHash common.Hash, firstKey []byte, lastKey []byte, keys [][]byte, values [][]byte, proof ethdb.KeyValueReader) (error, bool) {
	if len(keys) != len(values) {
		return fmt.Errorf("inconsistent proof data, keys: %d, values: %d", len(keys), len(values)), false
	}
	
	for i := 0; i < len(keys)-1; i++ {
		if bytes.Compare(keys[i], keys[i+1]) >= 0 {
			return errors.New("range is not monotonically increasing"), false
		}
	}
	
	
	if proof == nil {
		emptytrie, err := New(common.Hash{}, NewDatabase(memorydb.New()))
		if err != nil {
			return err, false
		}
		for index, key := range keys {
			emptytrie.TryUpdate(key, values[index])
		}
		if emptytrie.Hash() != rootHash {
			return fmt.Errorf("invalid proof, want hash %x, got %x", rootHash, emptytrie.Hash()), false
		}
		return nil, false 
	}
	
	
	if len(keys) == 0 {
		root, val, err := proofToPath(rootHash, nil, firstKey, proof, true)
		if err != nil {
			return err, false
		}
		if val != nil || hasRightElement(root, firstKey) {
			return errors.New("more entries available"), false
		}
		return nil, false
	}
	
	
	if len(keys) == 1 && bytes.Equal(firstKey, lastKey) {
		root, val, err := proofToPath(rootHash, nil, firstKey, proof, false)
		if err != nil {
			return err, false
		}
		if !bytes.Equal(firstKey, keys[0]) {
			return errors.New("correct proof but invalid key"), false
		}
		if !bytes.Equal(val, values[0]) {
			return errors.New("correct proof but invalid data"), false
		}
		return nil, hasRightElement(root, firstKey)
	}
	
	
	if bytes.Compare(firstKey, lastKey) >= 0 {
		return errors.New("invalid edge keys"), false
	}
	
	if len(firstKey) != len(lastKey) {
		return errors.New("inconsistent edge keys"), false
	}
	
	
	
	root, _, err := proofToPath(rootHash, nil, firstKey, proof, true)
	if err != nil {
		return err, false
	}
	
	
	
	root, _, err = proofToPath(rootHash, root, lastKey, proof, true)
	if err != nil {
		return err, false
	}
	
	
	if err := unsetInternal(root, firstKey, lastKey); err != nil {
		return err, false
	}
	
	
	newtrie := &Trie{root: root, db: NewDatabase(memorydb.New())}
	for index, key := range keys {
		newtrie.TryUpdate(key, values[index])
	}
	if newtrie.Hash() != rootHash {
		return fmt.Errorf("invalid proof, want hash %x, got %x", rootHash, newtrie.Hash()), false
	}
	return nil, hasRightElement(root, keys[len(keys)-1])
}






func get(tn node, key []byte, skipResolved bool) ([]byte, node) {
	for {
		switch n := tn.(type) {
		case *shortNode:
			if len(key) < len(n.Key) || !bytes.Equal(n.Key, key[:len(n.Key)]) {
				return nil, nil
			}
			tn = n.Val
			key = key[len(n.Key):]
			if !skipResolved {
				return key, tn
			}
		case *fullNode:
			tn = n.Children[key[0]]
			key = key[1:]
			if !skipResolved {
				return key, tn
			}
		case hashNode:
			return key, n
		case nil:
			return key, nil
		case valueNode:
			return nil, n
		default:
			panic(fmt.Sprintf("%T: invalid node: %v", tn, tn))
		}
	}
}
