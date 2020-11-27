















package bloombits

import (
	"errors"

	"github.com/ethereum/go-ethereum/core/types"
)

var (
	
	
	errSectionOutOfBounds = errors.New("section out of bounds")

	
	
	errBloomBitOutOfBounds = errors.New("bloom bit out of bounds")
)



type Generator struct {
	blooms   [types.BloomBitLength][]byte 
	sections uint                         
	nextSec  uint                         
}



func NewGenerator(sections uint) (*Generator, error) {
	if sections%8 != 0 {
		return nil, errors.New("section count not multiple of 8")
	}
	b := &Generator{sections: sections}
	for i := 0; i < types.BloomBitLength; i++ {
		b.blooms[i] = make([]byte, sections/8)
	}
	return b, nil
}



func (b *Generator) AddBloom(index uint, bloom types.Bloom) error {
	
	if b.nextSec >= b.sections {
		return errSectionOutOfBounds
	}
	if b.nextSec != index {
		return errors.New("bloom filter with unexpected index")
	}
	
	byteIndex := b.nextSec / 8
	bitIndex := byte(7 - b.nextSec%8)
	for byt := 0; byt < types.BloomByteLength; byt++ {
		bloomByte := bloom[types.BloomByteLength-1-byt]
		if bloomByte == 0 {
			continue
		}
		base := 8 * byt
		b.blooms[base+7][byteIndex] |= ((bloomByte >> 7) & 1) << bitIndex
		b.blooms[base+6][byteIndex] |= ((bloomByte >> 6) & 1) << bitIndex
		b.blooms[base+5][byteIndex] |= ((bloomByte >> 5) & 1) << bitIndex
		b.blooms[base+4][byteIndex] |= ((bloomByte >> 4) & 1) << bitIndex
		b.blooms[base+3][byteIndex] |= ((bloomByte >> 3) & 1) << bitIndex
		b.blooms[base+2][byteIndex] |= ((bloomByte >> 2) & 1) << bitIndex
		b.blooms[base+1][byteIndex] |= ((bloomByte >> 1) & 1) << bitIndex
		b.blooms[base][byteIndex] |= (bloomByte & 1) << bitIndex
	}
	b.nextSec++
	return nil
}



func (b *Generator) Bitset(idx uint) ([]byte, error) {
	if b.nextSec != b.sections {
		return nil, errors.New("bloom not fully generated yet")
	}
	if idx >= types.BloomBitLength {
		return nil, errBloomBitOutOfBounds
	}
	return b.blooms[idx], nil
}
