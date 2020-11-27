















package utils

import (
	"math"
	"sync"

	"github.com/ethereum/go-ethereum/common/mclock"
)



















type ExpiredValue struct {
	Base, Exp uint64 
}







type ExpirationFactor struct {
	Exp    uint64
	Factor float64
}


func ExpFactor(logOffset Fixed64) ExpirationFactor {
	return ExpirationFactor{Exp: logOffset.ToUint64(), Factor: logOffset.Fraction().Pow2()}
}



func (e ExpirationFactor) Value(base float64, exp uint64) float64 {
	return base / e.Factor * math.Pow(2, float64(int64(exp-e.Exp)))
}


func (e ExpiredValue) Value(logOffset Fixed64) uint64 {
	offset := Uint64ToFixed64(e.Exp) - logOffset
	return uint64(float64(e.Base) * offset.Pow2())
}


func (e *ExpiredValue) Add(amount int64, logOffset Fixed64) int64 {
	integer, frac := logOffset.ToUint64(), logOffset.Fraction()
	factor := frac.Pow2()
	base := factor * float64(amount)
	if integer < e.Exp {
		base /= math.Pow(2, float64(e.Exp-integer))
	}
	if integer > e.Exp {
		e.Base >>= (integer - e.Exp)
		e.Exp = integer
	}
	if base >= 0 || uint64(-base) <= e.Base {
		
		
		
		e.Base = uint64(int64(e.Base) + int64(base))
		return amount
	}
	net := int64(-float64(e.Base) / factor)
	e.Base = 0
	return net
}


func (e *ExpiredValue) AddExp(a ExpiredValue) {
	if e.Exp > a.Exp {
		a.Base >>= (e.Exp - a.Exp)
	}
	if e.Exp < a.Exp {
		e.Base >>= (a.Exp - e.Exp)
		e.Exp = a.Exp
	}
	e.Base += a.Base
}


func (e *ExpiredValue) SubExp(a ExpiredValue) {
	if e.Exp > a.Exp {
		a.Base >>= (e.Exp - a.Exp)
	}
	if e.Exp < a.Exp {
		e.Base >>= (a.Exp - e.Exp)
		e.Exp = a.Exp
	}
	if e.Base > a.Base {
		e.Base -= a.Base
	} else {
		e.Base = 0
	}
}


func (e *ExpiredValue) IsZero() bool {
	return e.Base == 0
}



type LinearExpiredValue struct {
	Offset uint64         
	Val    uint64         
	Rate   mclock.AbsTime `rlp:"-"` 
}



func (e LinearExpiredValue) Value(now mclock.AbsTime) uint64 {
	offset := uint64(now / e.Rate)
	if e.Offset < offset {
		diff := offset - e.Offset
		if e.Val >= diff {
			e.Val -= diff
		} else {
			e.Val = 0
		}
	}
	return e.Val
}



func (e *LinearExpiredValue) Add(amount int64, now mclock.AbsTime) uint64 {
	offset := uint64(now / e.Rate)
	if e.Offset < offset {
		diff := offset - e.Offset
		if e.Val >= diff {
			e.Val -= diff
		} else {
			e.Val = 0
		}
		e.Offset = offset
	}
	if amount < 0 && uint64(-amount) > e.Val {
		e.Val = 0
	} else {
		e.Val = uint64(int64(e.Val) + amount)
	}
	return e.Val
}


type ValueExpirer interface {
	SetRate(now mclock.AbsTime, rate float64)
	SetLogOffset(now mclock.AbsTime, logOffset Fixed64)
	LogOffset(now mclock.AbsTime) Fixed64
}






type Expirer struct {
	lock       sync.RWMutex
	logOffset  Fixed64
	rate       float64
	lastUpdate mclock.AbsTime
}



func (e *Expirer) SetRate(now mclock.AbsTime, rate float64) {
	e.lock.Lock()
	defer e.lock.Unlock()

	dt := now - e.lastUpdate
	if dt > 0 {
		e.logOffset += Fixed64(logToFixedFactor * float64(dt) * e.rate)
	}
	e.lastUpdate = now
	e.rate = rate
}


func (e *Expirer) SetLogOffset(now mclock.AbsTime, logOffset Fixed64) {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.lastUpdate = now
	e.logOffset = logOffset
}


func (e *Expirer) LogOffset(now mclock.AbsTime) Fixed64 {
	e.lock.RLock()
	defer e.lock.RUnlock()

	dt := now - e.lastUpdate
	if dt <= 0 {
		return e.logOffset
	}
	return e.logOffset + Fixed64(logToFixedFactor*float64(dt)*e.rate)
}


const fixedFactor = 0x1000000


type Fixed64 int64


func Uint64ToFixed64(f uint64) Fixed64 {
	return Fixed64(f * fixedFactor)
}


func Float64ToFixed64(f float64) Fixed64 {
	return Fixed64(f * fixedFactor)
}


func (f64 Fixed64) ToUint64() uint64 {
	return uint64(f64) / fixedFactor
}


func (f64 Fixed64) Fraction() Fixed64 {
	return f64 % fixedFactor
}

var (
	logToFixedFactor = float64(fixedFactor) / math.Log(2)
	fixedToLogFactor = math.Log(2) / float64(fixedFactor)
)


func (f64 Fixed64) Pow2() float64 {
	return math.Exp(float64(f64) * fixedToLogFactor)
}
