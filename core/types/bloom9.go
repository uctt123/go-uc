















package types

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type bytesBacked interface {
	Bytes() []byte
}

const (
	
	BloomByteLength = 256

	
	BloomBitLength = 8 * BloomByteLength
)


type Bloom [BloomByteLength]byte



func BytesToBloom(b []byte) Bloom {
	var bloom Bloom
	bloom.SetBytes(b)
	return bloom
}



func (b *Bloom) SetBytes(d []byte) {
	if len(b) < len(d) {
		panic(fmt.Sprintf("bloom bytes too big %d %d", len(b), len(d)))
	}
	copy(b[BloomByteLength-len(d):], d)
}


func (b *Bloom) Add(d []byte) {
	b.add(d, make([]byte, 6))
}


func (b *Bloom) add(d []byte, buf []byte) {
	i1, v1, i2, v2, i3, v3 := bloomValues(d, buf)
	b[i1] |= v1
	b[i2] |= v2
	b[i3] |= v3
}




func (b Bloom) Big() *big.Int {
	return new(big.Int).SetBytes(b[:])
}


func (b Bloom) Bytes() []byte {
	return b[:]
}


func (b Bloom) Test(topic []byte) bool {
	i1, v1, i2, v2, i3, v3 := bloomValues(topic, make([]byte, 6))
	return v1 == v1&b[i1] &&
		v2 == v2&b[i2] &&
		v3 == v3&b[i3]
}


func (b Bloom) MarshalText() ([]byte, error) {
	return hexutil.Bytes(b[:]).MarshalText()
}


func (b *Bloom) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("Bloom", input, b[:])
}


func CreateBloom(receipts Receipts) Bloom {
	buf := make([]byte, 6)
	var bin Bloom
	for _, receipt := range receipts {
		for _, log := range receipt.Logs {
			bin.add(log.Address.Bytes(), buf)
			for _, b := range log.Topics {
				bin.add(b[:], buf)
			}
		}
	}
	return bin
}


func LogsBloom(logs []*Log) []byte {
	buf := make([]byte, 6)
	var bin Bloom
	for _, log := range logs {
		bin.add(log.Address.Bytes(), buf)
		for _, b := range log.Topics {
			bin.add(b[:], buf)
		}
	}
	return bin[:]
}


func Bloom9(data []byte) []byte {
	var b Bloom
	b.SetBytes(data)
	return b.Bytes()
}


func bloomValues(data []byte, hashbuf []byte) (uint, byte, uint, byte, uint, byte) {
	sha := hasherPool.Get().(crypto.KeccakState)
	sha.Reset()
	sha.Write(data)
	sha.Read(hashbuf)
	hasherPool.Put(sha)
	
	v1 := byte(1 << (hashbuf[1] & 0x7))
	v2 := byte(1 << (hashbuf[3] & 0x7))
	v3 := byte(1 << (hashbuf[5] & 0x7))
	
	i1 := BloomByteLength - uint((binary.BigEndian.Uint16(hashbuf)&0x7ff)>>3) - 1
	i2 := BloomByteLength - uint((binary.BigEndian.Uint16(hashbuf[2:])&0x7ff)>>3) - 1
	i3 := BloomByteLength - uint((binary.BigEndian.Uint16(hashbuf[4:])&0x7ff)>>3) - 1

	return i1, v1, i2, v2, i3, v3
}


func BloomLookup(bin Bloom, topic bytesBacked) bool {
	return bin.Test(topic.Bytes())
}
