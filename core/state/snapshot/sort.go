















package snapshot

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
)


type hashes []common.Hash


func (hs hashes) Len() int { return len(hs) }



func (hs hashes) Less(i, j int) bool { return bytes.Compare(hs[i][:], hs[j][:]) < 0 }


func (hs hashes) Swap(i, j int) { hs[i], hs[j] = hs[j], hs[i] }
