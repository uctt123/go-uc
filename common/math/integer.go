















package math

import (
	"fmt"
	"math/bits"
	"strconv"
)


const (
	MaxInt8   = 1<<7 - 1
	MinInt8   = -1 << 7
	MaxInt16  = 1<<15 - 1
	MinInt16  = -1 << 15
	MaxInt32  = 1<<31 - 1
	MinInt32  = -1 << 31
	MaxInt64  = 1<<63 - 1
	MinInt64  = -1 << 63
	MaxUint8  = 1<<8 - 1
	MaxUint16 = 1<<16 - 1
	MaxUint32 = 1<<32 - 1
	MaxUint64 = 1<<64 - 1
)


type HexOrDecimal64 uint64


func (i *HexOrDecimal64) UnmarshalText(input []byte) error {
	int, ok := ParseUint64(string(input))
	if !ok {
		return fmt.Errorf("invalid hex or decimal integer %q", input)
	}
	*i = HexOrDecimal64(int)
	return nil
}


func (i HexOrDecimal64) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%#x", uint64(i))), nil
}



func ParseUint64(s string) (uint64, bool) {
	if s == "" {
		return 0, true
	}
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		v, err := strconv.ParseUint(s[2:], 16, 64)
		return v, err == nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	return v, err == nil
}


func MustParseUint64(s string) uint64 {
	v, ok := ParseUint64(s)
	if !ok {
		panic("invalid unsigned 64 bit integer: " + s)
	}
	return v
}


func SafeSub(x, y uint64) (uint64, bool) {
	diff, borrowOut := bits.Sub64(x, y, 0)
	return diff, borrowOut != 0
}


func SafeAdd(x, y uint64) (uint64, bool) {
	sum, carryOut := bits.Add64(x, y, 0)
	return sum, carryOut != 0
}


func SafeMul(x, y uint64) (uint64, bool) {
	hi, lo := bits.Mul64(x, y)
	return lo, hi != 0
}
