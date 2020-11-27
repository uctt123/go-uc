















package trie

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)




type MissingNodeError struct {
	NodeHash common.Hash 
	Path     []byte      
}

func (err *MissingNodeError) Error() string {
	return fmt.Sprintf("missing trie node %x (path %x)", err.NodeHash, err.Path)
}
