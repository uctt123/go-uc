















package types

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

type DerivableList interface {
	Len() int
	GetRlp(i int) []byte
}


type Hasher interface {
	Reset()
	Update([]byte, []byte)
	Hash() common.Hash
}

func DeriveSha(list DerivableList, hasher Hasher) common.Hash {
	hasher.Reset()
	keybuf := new(bytes.Buffer)

	
	
	
	
	for i := 1; i < list.Len() && i <= 0x7f; i++ {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(i))
		hasher.Update(keybuf.Bytes(), list.GetRlp(i))
	}
	if list.Len() > 0 {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(0))
		hasher.Update(keybuf.Bytes(), list.GetRlp(0))
	}
	for i := 0x80; i < list.Len(); i++ {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(i))
		hasher.Update(keybuf.Bytes(), list.GetRlp(i))
	}
	return hasher.Hash()
}
