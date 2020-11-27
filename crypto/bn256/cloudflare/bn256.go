












package bn256

import (
	"crypto/rand"
	"errors"
	"io"
	"math/big"
)

func randomK(r io.Reader) (k *big.Int, err error) {
	for {
		k, err = rand.Int(r, Order)
		if k.Sign() > 0 || err != nil {
			return
		}
	}
}



type G1 struct {
	p *curvePoint
}


func RandomG1(r io.Reader) (*big.Int, *G1, error) {
	k, err := randomK(r)
	if err != nil {
		return nil, nil, err
	}

	return k, new(G1).ScalarBaseMult(k), nil
}

func (g *G1) String() string {
	return "bn256.G1" + g.p.String()
}



func (e *G1) ScalarBaseMult(k *big.Int) *G1 {
	if e.p == nil {
		e.p = &curvePoint{}
	}
	e.p.Mul(curveGen, k)
	return e
}


func (e *G1) ScalarMult(a *G1, k *big.Int) *G1 {
	if e.p == nil {
		e.p = &curvePoint{}
	}
	e.p.Mul(a.p, k)
	return e
}


func (e *G1) Add(a, b *G1) *G1 {
	if e.p == nil {
		e.p = &curvePoint{}
	}
	e.p.Add(a.p, b.p)
	return e
}


func (e *G1) Neg(a *G1) *G1 {
	if e.p == nil {
		e.p = &curvePoint{}
	}
	e.p.Neg(a.p)
	return e
}


func (e *G1) Set(a *G1) *G1 {
	if e.p == nil {
		e.p = &curvePoint{}
	}
	e.p.Set(a.p)
	return e
}


func (e *G1) Marshal() []byte {
	
	const numBytes = 256 / 8

	if e.p == nil {
		e.p = &curvePoint{}
	}

	e.p.MakeAffine()
	ret := make([]byte, numBytes*2)
	if e.p.IsInfinity() {
		return ret
	}
	temp := &gfP{}

	montDecode(temp, &e.p.x)
	temp.Marshal(ret)
	montDecode(temp, &e.p.y)
	temp.Marshal(ret[numBytes:])

	return ret
}



func (e *G1) Unmarshal(m []byte) ([]byte, error) {
	
	const numBytes = 256 / 8
	if len(m) < 2*numBytes {
		return nil, errors.New("bn256: not enough data")
	}
	
	if e.p == nil {
		e.p = &curvePoint{}
	} else {
		e.p.x, e.p.y = gfP{0}, gfP{0}
	}
	var err error
	if err = e.p.x.Unmarshal(m); err != nil {
		return nil, err
	}
	if err = e.p.y.Unmarshal(m[numBytes:]); err != nil {
		return nil, err
	}
	
	montEncode(&e.p.x, &e.p.x)
	montEncode(&e.p.y, &e.p.y)

	zero := gfP{0}
	if e.p.x == zero && e.p.y == zero {
		
		e.p.y = *newGFp(1)
		e.p.z = gfP{0}
		e.p.t = gfP{0}
	} else {
		e.p.z = *newGFp(1)
		e.p.t = *newGFp(1)

		if !e.p.IsOnCurve() {
			return nil, errors.New("bn256: malformed point")
		}
	}
	return m[2*numBytes:], nil
}



type G2 struct {
	p *twistPoint
}


func RandomG2(r io.Reader) (*big.Int, *G2, error) {
	k, err := randomK(r)
	if err != nil {
		return nil, nil, err
	}

	return k, new(G2).ScalarBaseMult(k), nil
}

func (e *G2) String() string {
	return "bn256.G2" + e.p.String()
}



func (e *G2) ScalarBaseMult(k *big.Int) *G2 {
	if e.p == nil {
		e.p = &twistPoint{}
	}
	e.p.Mul(twistGen, k)
	return e
}


func (e *G2) ScalarMult(a *G2, k *big.Int) *G2 {
	if e.p == nil {
		e.p = &twistPoint{}
	}
	e.p.Mul(a.p, k)
	return e
}


func (e *G2) Add(a, b *G2) *G2 {
	if e.p == nil {
		e.p = &twistPoint{}
	}
	e.p.Add(a.p, b.p)
	return e
}


func (e *G2) Neg(a *G2) *G2 {
	if e.p == nil {
		e.p = &twistPoint{}
	}
	e.p.Neg(a.p)
	return e
}


func (e *G2) Set(a *G2) *G2 {
	if e.p == nil {
		e.p = &twistPoint{}
	}
	e.p.Set(a.p)
	return e
}


func (e *G2) Marshal() []byte {
	
	const numBytes = 256 / 8

	if e.p == nil {
		e.p = &twistPoint{}
	}

	e.p.MakeAffine()
	ret := make([]byte, numBytes*4)
	if e.p.IsInfinity() {
		return ret
	}
	temp := &gfP{}

	montDecode(temp, &e.p.x.x)
	temp.Marshal(ret)
	montDecode(temp, &e.p.x.y)
	temp.Marshal(ret[numBytes:])
	montDecode(temp, &e.p.y.x)
	temp.Marshal(ret[2*numBytes:])
	montDecode(temp, &e.p.y.y)
	temp.Marshal(ret[3*numBytes:])

	return ret
}



func (e *G2) Unmarshal(m []byte) ([]byte, error) {
	
	const numBytes = 256 / 8
	if len(m) < 4*numBytes {
		return nil, errors.New("bn256: not enough data")
	}
	
	if e.p == nil {
		e.p = &twistPoint{}
	}
	var err error
	if err = e.p.x.x.Unmarshal(m); err != nil {
		return nil, err
	}
	if err = e.p.x.y.Unmarshal(m[numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.x.Unmarshal(m[2*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.y.Unmarshal(m[3*numBytes:]); err != nil {
		return nil, err
	}
	
	montEncode(&e.p.x.x, &e.p.x.x)
	montEncode(&e.p.x.y, &e.p.x.y)
	montEncode(&e.p.y.x, &e.p.y.x)
	montEncode(&e.p.y.y, &e.p.y.y)

	if e.p.x.IsZero() && e.p.y.IsZero() {
		
		e.p.y.SetOne()
		e.p.z.SetZero()
		e.p.t.SetZero()
	} else {
		e.p.z.SetOne()
		e.p.t.SetOne()

		if !e.p.IsOnCurve() {
			return nil, errors.New("bn256: malformed point")
		}
	}
	return m[4*numBytes:], nil
}



type GT struct {
	p *gfP12
}


func Pair(g1 *G1, g2 *G2) *GT {
	return &GT{optimalAte(g2.p, g1.p)}
}


func PairingCheck(a []*G1, b []*G2) bool {
	acc := new(gfP12)
	acc.SetOne()

	for i := 0; i < len(a); i++ {
		if a[i].p.IsInfinity() || b[i].p.IsInfinity() {
			continue
		}
		acc.Mul(acc, miller(b[i].p, a[i].p))
	}
	return finalExponentiation(acc).IsOne()
}




func Miller(g1 *G1, g2 *G2) *GT {
	return &GT{miller(g2.p, g1.p)}
}

func (g *GT) String() string {
	return "bn256.GT" + g.p.String()
}


func (e *GT) ScalarMult(a *GT, k *big.Int) *GT {
	if e.p == nil {
		e.p = &gfP12{}
	}
	e.p.Exp(a.p, k)
	return e
}


func (e *GT) Add(a, b *GT) *GT {
	if e.p == nil {
		e.p = &gfP12{}
	}
	e.p.Mul(a.p, b.p)
	return e
}


func (e *GT) Neg(a *GT) *GT {
	if e.p == nil {
		e.p = &gfP12{}
	}
	e.p.Conjugate(a.p)
	return e
}


func (e *GT) Set(a *GT) *GT {
	if e.p == nil {
		e.p = &gfP12{}
	}
	e.p.Set(a.p)
	return e
}


func (e *GT) Finalize() *GT {
	ret := finalExponentiation(e.p)
	e.p.Set(ret)
	return e
}


func (e *GT) Marshal() []byte {
	
	const numBytes = 256 / 8

	if e.p == nil {
		e.p = &gfP12{}
		e.p.SetOne()
	}

	ret := make([]byte, numBytes*12)
	temp := &gfP{}

	montDecode(temp, &e.p.x.x.x)
	temp.Marshal(ret)
	montDecode(temp, &e.p.x.x.y)
	temp.Marshal(ret[numBytes:])
	montDecode(temp, &e.p.x.y.x)
	temp.Marshal(ret[2*numBytes:])
	montDecode(temp, &e.p.x.y.y)
	temp.Marshal(ret[3*numBytes:])
	montDecode(temp, &e.p.x.z.x)
	temp.Marshal(ret[4*numBytes:])
	montDecode(temp, &e.p.x.z.y)
	temp.Marshal(ret[5*numBytes:])
	montDecode(temp, &e.p.y.x.x)
	temp.Marshal(ret[6*numBytes:])
	montDecode(temp, &e.p.y.x.y)
	temp.Marshal(ret[7*numBytes:])
	montDecode(temp, &e.p.y.y.x)
	temp.Marshal(ret[8*numBytes:])
	montDecode(temp, &e.p.y.y.y)
	temp.Marshal(ret[9*numBytes:])
	montDecode(temp, &e.p.y.z.x)
	temp.Marshal(ret[10*numBytes:])
	montDecode(temp, &e.p.y.z.y)
	temp.Marshal(ret[11*numBytes:])

	return ret
}



func (e *GT) Unmarshal(m []byte) ([]byte, error) {
	
	const numBytes = 256 / 8

	if len(m) < 12*numBytes {
		return nil, errors.New("bn256: not enough data")
	}

	if e.p == nil {
		e.p = &gfP12{}
	}

	var err error
	if err = e.p.x.x.x.Unmarshal(m); err != nil {
		return nil, err
	}
	if err = e.p.x.x.y.Unmarshal(m[numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.x.y.x.Unmarshal(m[2*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.x.y.y.Unmarshal(m[3*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.x.z.x.Unmarshal(m[4*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.x.z.y.Unmarshal(m[5*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.x.x.Unmarshal(m[6*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.x.y.Unmarshal(m[7*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.y.x.Unmarshal(m[8*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.y.y.Unmarshal(m[9*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.z.x.Unmarshal(m[10*numBytes:]); err != nil {
		return nil, err
	}
	if err = e.p.y.z.y.Unmarshal(m[11*numBytes:]); err != nil {
		return nil, err
	}
	montEncode(&e.p.x.x.x, &e.p.x.x.x)
	montEncode(&e.p.x.x.y, &e.p.x.x.y)
	montEncode(&e.p.x.y.x, &e.p.x.y.x)
	montEncode(&e.p.x.y.y, &e.p.x.y.y)
	montEncode(&e.p.x.z.x, &e.p.x.z.x)
	montEncode(&e.p.x.z.y, &e.p.x.z.y)
	montEncode(&e.p.y.x.x, &e.p.y.x.x)
	montEncode(&e.p.y.x.y, &e.p.y.x.y)
	montEncode(&e.p.y.y.x, &e.p.y.y.x)
	montEncode(&e.p.y.y.y, &e.p.y.y.y)
	montEncode(&e.p.y.z.x, &e.p.y.z.x)
	montEncode(&e.p.y.z.y, &e.p.y.z.y)

	return m[12*numBytes:], nil
}
