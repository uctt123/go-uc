















package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/holiman/uint256"
)



func calcMemSize64(off, l *uint256.Int) (uint64, bool) {
	if !l.IsUint64() {
		return 0, true
	}
	return calcMemSize64WithUint(off, l.Uint64())
}




func calcMemSize64WithUint(off *uint256.Int, length64 uint64) (uint64, bool) {
	
	if length64 == 0 {
		return 0, false
	}
	
	offset64, overflow := off.Uint64WithOverflow()
	if overflow {
		return 0, true
	}
	val := offset64 + length64
	
	return val, val < offset64
}



func getData(data []byte, start uint64, size uint64) []byte {
	length := uint64(len(data))
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return common.RightPadBytes(data[start:end], int(size))
}


func toWordSize(size uint64) uint64 {
	if size > math.MaxUint64-31 {
		return math.MaxUint64/32 + 1
	}

	return (size + 31) / 32
}

func allZero(b []byte) bool {
	for _, byte := range b {
		if byte != 0 {
			return false
		}
	}
	return true
}
