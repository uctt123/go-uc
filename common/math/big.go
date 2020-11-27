
















package math

import (
	"fmt"
	"math/big"
)


var (
	tt255     = BigPow(2, 255)
	tt256     = BigPow(2, 256)
	tt256m1   = new(big.Int).Sub(tt256, big.NewInt(1))
	tt63      = BigPow(2, 63)
	MaxBig256 = new(big.Int).Set(tt256m1)
	MaxBig63  = new(big.Int).Sub(tt63, big.NewInt(1))
)

const (
	
	wordBits = 32 << (uint64(^big.Word(0)) >> 63)
	
	wordBytes = wordBits / 8
)


type HexOrDecimal256 big.Int


func NewHexOrDecimal256(x int64) *HexOrDecimal256 {
	b := big.NewInt(x)
	h := HexOrDecimal256(*b)
	return &h
}


func (i *HexOrDecimal256) UnmarshalText(input []byte) error {
	bigint, ok := ParseBig256(string(input))
	if !ok {
		return fmt.Errorf("invalid hex or decimal integer %q", input)
	}
	*i = HexOrDecimal256(*bigint)
	return nil
}


func (i *HexOrDecimal256) MarshalText() ([]byte, error) {
	if i == nil {
		return []byte("0x0"), nil
	}
	return []byte(fmt.Sprintf("%#x", (*big.Int)(i))), nil
}



type Decimal256 big.Int


func NewDecimal256(x int64) *Decimal256 {
	b := big.NewInt(x)
	d := Decimal256(*b)
	return &d
}


func (i *Decimal256) UnmarshalText(input []byte) error {
	bigint, ok := ParseBig256(string(input))
	if !ok {
		return fmt.Errorf("invalid hex or decimal integer %q", input)
	}
	*i = Decimal256(*bigint)
	return nil
}


func (i *Decimal256) MarshalText() ([]byte, error) {
	return []byte(i.String()), nil
}


func (i *Decimal256) String() string {
	if i == nil {
		return "0"
	}
	return fmt.Sprintf("%#d", (*big.Int)(i))
}



func ParseBig256(s string) (*big.Int, bool) {
	if s == "" {
		return new(big.Int), true
	}
	var bigint *big.Int
	var ok bool
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		bigint, ok = new(big.Int).SetString(s[2:], 16)
	} else {
		bigint, ok = new(big.Int).SetString(s, 10)
	}
	if ok && bigint.BitLen() > 256 {
		bigint, ok = nil, false
	}
	return bigint, ok
}


func MustParseBig256(s string) *big.Int {
	v, ok := ParseBig256(s)
	if !ok {
		panic("invalid 256 bit integer: " + s)
	}
	return v
}


func BigPow(a, b int64) *big.Int {
	r := big.NewInt(a)
	return r.Exp(r, big.NewInt(b), nil)
}


func BigMax(x, y *big.Int) *big.Int {
	if x.Cmp(y) < 0 {
		return y
	}
	return x
}


func BigMin(x, y *big.Int) *big.Int {
	if x.Cmp(y) > 0 {
		return y
	}
	return x
}


func FirstBitSet(v *big.Int) int {
	for i := 0; i < v.BitLen(); i++ {
		if v.Bit(i) > 0 {
			return i
		}
	}
	return v.BitLen()
}



func PaddedBigBytes(bigint *big.Int, n int) []byte {
	if bigint.BitLen()/8 >= n {
		return bigint.Bytes()
	}
	ret := make([]byte, n)
	ReadBits(bigint, ret)
	return ret
}




func bigEndianByteAt(bigint *big.Int, n int) byte {
	words := bigint.Bits()
	
	i := n / wordBytes
	if i >= len(words) {
		return byte(0)
	}
	word := words[i]
	
	shift := 8 * uint(n%wordBytes)

	return byte(word >> shift)
}





func Byte(bigint *big.Int, padlength, n int) byte {
	if n >= padlength {
		return byte(0)
	}
	return bigEndianByteAt(bigint, padlength-1-n)
}



func ReadBits(bigint *big.Int, buf []byte) {
	i := len(buf)
	for _, d := range bigint.Bits() {
		for j := 0; j < wordBytes && i > 0; j++ {
			i--
			buf[i] = byte(d)
			d >>= 8
		}
	}
}


func U256(x *big.Int) *big.Int {
	return x.And(x, tt256m1)
}



func U256Bytes(n *big.Int) []byte {
	return PaddedBigBytes(U256(n), 32)
}








func S256(x *big.Int) *big.Int {
	if x.Cmp(tt255) < 0 {
		return x
	}
	return new(big.Int).Sub(x, tt256)
}






func Exp(base, exponent *big.Int) *big.Int {
	result := big.NewInt(1)

	for _, word := range exponent.Bits() {
		for i := 0; i < wordBits; i++ {
			if word&1 == 1 {
				U256(result.Mul(result, base))
			}
			U256(base.Mul(base, base))
			word >>= 1
		}
	}
	return result
}
