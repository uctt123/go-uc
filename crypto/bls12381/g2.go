















package bls12381

import (
	"errors"
	"math"
	"math/big"
)




type PointG2 [3]fe2


func (p *PointG2) Set(p2 *PointG2) *PointG2 {
	p[0].set(&p2[0])
	p[1].set(&p2[1])
	p[2].set(&p2[2])
	return p
}


func (p *PointG2) Zero() *PointG2 {
	p[0].zero()
	p[1].one()
	p[2].zero()
	return p

}

type tempG2 struct {
	t [9]*fe2
}


type G2 struct {
	f *fp2
	tempG2
}


func NewG2() *G2 {
	return newG2(nil)
}

func newG2(f *fp2) *G2 {
	if f == nil {
		f = newFp2()
	}
	t := newTempG2()
	return &G2{f, t}
}

func newTempG2() tempG2 {
	t := [9]*fe2{}
	for i := 0; i < 9; i++ {
		t[i] = &fe2{}
	}
	return tempG2{t}
}


func (g *G2) Q() *big.Int {
	return new(big.Int).Set(q)
}

func (g *G2) fromBytesUnchecked(in []byte) (*PointG2, error) {
	p0, err := g.f.fromBytes(in[:96])
	if err != nil {
		return nil, err
	}
	p1, err := g.f.fromBytes(in[96:])
	if err != nil {
		return nil, err
	}
	p2 := new(fe2).one()
	return &PointG2{*p0, *p1, *p2}, nil
}






func (g *G2) FromBytes(in []byte) (*PointG2, error) {
	if len(in) != 192 {
		return nil, errors.New("input string should be equal or larger than 192")
	}
	p0, err := g.f.fromBytes(in[:96])
	if err != nil {
		return nil, err
	}
	p1, err := g.f.fromBytes(in[96:])
	if err != nil {
		return nil, err
	}
	
	if p0.isZero() && p1.isZero() {
		return g.Zero(), nil
	}
	p2 := new(fe2).one()
	p := &PointG2{*p0, *p1, *p2}
	if !g.IsOnCurve(p) {
		return nil, errors.New("point is not on curve")
	}
	return p, nil
}


func (g *G2) DecodePoint(in []byte) (*PointG2, error) {
	if len(in) != 256 {
		return nil, errors.New("invalid g2 point length")
	}
	pointBytes := make([]byte, 192)
	x0Bytes, err := decodeFieldElement(in[:64])
	if err != nil {
		return nil, err
	}
	x1Bytes, err := decodeFieldElement(in[64:128])
	if err != nil {
		return nil, err
	}
	y0Bytes, err := decodeFieldElement(in[128:192])
	if err != nil {
		return nil, err
	}
	y1Bytes, err := decodeFieldElement(in[192:])
	if err != nil {
		return nil, err
	}
	copy(pointBytes[:48], x1Bytes)
	copy(pointBytes[48:96], x0Bytes)
	copy(pointBytes[96:144], y1Bytes)
	copy(pointBytes[144:192], y0Bytes)
	return g.FromBytes(pointBytes)
}




func (g *G2) ToBytes(p *PointG2) []byte {
	out := make([]byte, 192)
	if g.IsZero(p) {
		return out
	}
	g.Affine(p)
	copy(out[:96], g.f.toBytes(&p[0]))
	copy(out[96:], g.f.toBytes(&p[1]))
	return out
}


func (g *G2) EncodePoint(p *PointG2) []byte {
	
	outRaw := g.ToBytes(p)
	out := make([]byte, 256)
	
	copy(out[16:16+48], outRaw[48:96])
	copy(out[80:80+48], outRaw[:48])
	
	copy(out[144:144+48], outRaw[144:])
	copy(out[208:208+48], outRaw[96:144])
	return out
}


func (g *G2) New() *PointG2 {
	return new(PointG2).Zero()
}


func (g *G2) Zero() *PointG2 {
	return new(PointG2).Zero()
}


func (g *G2) One() *PointG2 {
	p := &PointG2{}
	return p.Set(&g2One)
}


func (g *G2) IsZero(p *PointG2) bool {
	return p[2].isZero()
}


func (g *G2) Equal(p1, p2 *PointG2) bool {
	if g.IsZero(p1) {
		return g.IsZero(p2)
	}
	if g.IsZero(p2) {
		return g.IsZero(p1)
	}
	t := g.t
	g.f.square(t[0], &p1[2])
	g.f.square(t[1], &p2[2])
	g.f.mul(t[2], t[0], &p2[0])
	g.f.mul(t[3], t[1], &p1[0])
	g.f.mul(t[0], t[0], &p1[2])
	g.f.mul(t[1], t[1], &p2[2])
	g.f.mul(t[1], t[1], &p1[1])
	g.f.mul(t[0], t[0], &p2[1])
	return t[0].equal(t[1]) && t[2].equal(t[3])
}


func (g *G2) InCorrectSubgroup(p *PointG2) bool {
	tmp := &PointG2{}
	g.MulScalar(tmp, p, q)
	return g.IsZero(tmp)
}


func (g *G2) IsOnCurve(p *PointG2) bool {
	if g.IsZero(p) {
		return true
	}
	t := g.t
	g.f.square(t[0], &p[1])
	g.f.square(t[1], &p[0])
	g.f.mul(t[1], t[1], &p[0])
	g.f.square(t[2], &p[2])
	g.f.square(t[3], t[2])
	g.f.mul(t[2], t[2], t[3])
	g.f.mul(t[2], b2, t[2])
	g.f.add(t[1], t[1], t[2])
	return t[0].equal(t[1])
}


func (g *G2) IsAffine(p *PointG2) bool {
	return p[2].isOne()
}


func (g *G2) Affine(p *PointG2) *PointG2 {
	if g.IsZero(p) {
		return p
	}
	if !g.IsAffine(p) {
		t := g.t
		g.f.inverse(t[0], &p[2])
		g.f.square(t[1], t[0])
		g.f.mul(&p[0], &p[0], t[1])
		g.f.mul(t[0], t[0], t[1])
		g.f.mul(&p[1], &p[1], t[0])
		p[2].one()
	}
	return p
}


func (g *G2) Add(r, p1, p2 *PointG2) *PointG2 {
	
	if g.IsZero(p1) {
		return r.Set(p2)
	}
	if g.IsZero(p2) {
		return r.Set(p1)
	}
	t := g.t
	g.f.square(t[7], &p1[2])
	g.f.mul(t[1], &p2[0], t[7])
	g.f.mul(t[2], &p1[2], t[7])
	g.f.mul(t[0], &p2[1], t[2])
	g.f.square(t[8], &p2[2])
	g.f.mul(t[3], &p1[0], t[8])
	g.f.mul(t[4], &p2[2], t[8])
	g.f.mul(t[2], &p1[1], t[4])
	if t[1].equal(t[3]) {
		if t[0].equal(t[2]) {
			return g.Double(r, p1)
		} else {
			return r.Zero()
		}
	}
	g.f.sub(t[1], t[1], t[3])
	g.f.double(t[4], t[1])
	g.f.square(t[4], t[4])
	g.f.mul(t[5], t[1], t[4])
	g.f.sub(t[0], t[0], t[2])
	g.f.double(t[0], t[0])
	g.f.square(t[6], t[0])
	g.f.sub(t[6], t[6], t[5])
	g.f.mul(t[3], t[3], t[4])
	g.f.double(t[4], t[3])
	g.f.sub(&r[0], t[6], t[4])
	g.f.sub(t[4], t[3], &r[0])
	g.f.mul(t[6], t[2], t[5])
	g.f.double(t[6], t[6])
	g.f.mul(t[0], t[0], t[4])
	g.f.sub(&r[1], t[0], t[6])
	g.f.add(t[0], &p1[2], &p2[2])
	g.f.square(t[0], t[0])
	g.f.sub(t[0], t[0], t[7])
	g.f.sub(t[0], t[0], t[8])
	g.f.mul(&r[2], t[0], t[1])
	return r
}


func (g *G2) Double(r, p *PointG2) *PointG2 {
	
	if g.IsZero(p) {
		return r.Set(p)
	}
	t := g.t
	g.f.square(t[0], &p[0])
	g.f.square(t[1], &p[1])
	g.f.square(t[2], t[1])
	g.f.add(t[1], &p[0], t[1])
	g.f.square(t[1], t[1])
	g.f.sub(t[1], t[1], t[0])
	g.f.sub(t[1], t[1], t[2])
	g.f.double(t[1], t[1])
	g.f.double(t[3], t[0])
	g.f.add(t[0], t[3], t[0])
	g.f.square(t[4], t[0])
	g.f.double(t[3], t[1])
	g.f.sub(&r[0], t[4], t[3])
	g.f.sub(t[1], t[1], &r[0])
	g.f.double(t[2], t[2])
	g.f.double(t[2], t[2])
	g.f.double(t[2], t[2])
	g.f.mul(t[0], t[0], t[1])
	g.f.sub(t[1], t[0], t[2])
	g.f.mul(t[0], &p[1], &p[2])
	r[1].set(t[1])
	g.f.double(&r[2], t[0])
	return r
}


func (g *G2) Neg(r, p *PointG2) *PointG2 {
	r[0].set(&p[0])
	g.f.neg(&r[1], &p[1])
	r[2].set(&p[2])
	return r
}


func (g *G2) Sub(c, a, b *PointG2) *PointG2 {
	d := &PointG2{}
	g.Neg(d, b)
	g.Add(c, a, d)
	return c
}


func (g *G2) MulScalar(c, p *PointG2, e *big.Int) *PointG2 {
	q, n := &PointG2{}, &PointG2{}
	n.Set(p)
	l := e.BitLen()
	for i := 0; i < l; i++ {
		if e.Bit(i) == 1 {
			g.Add(q, q, n)
		}
		g.Double(n, n)
	}
	return c.Set(q)
}


func (g *G2) ClearCofactor(p *PointG2) {
	g.MulScalar(p, p, cofactorEFFG2)
}





func (g *G2) MultiExp(r *PointG2, points []*PointG2, powers []*big.Int) (*PointG2, error) {
	if len(points) != len(powers) {
		return nil, errors.New("point and scalar vectors should be in same length")
	}
	var c uint32 = 3
	if len(powers) >= 32 {
		c = uint32(math.Ceil(math.Log10(float64(len(powers)))))
	}
	bucketSize, numBits := (1<<c)-1, uint32(g.Q().BitLen())
	windows := make([]*PointG2, numBits/c+1)
	bucket := make([]*PointG2, bucketSize)
	acc, sum := g.New(), g.New()
	for i := 0; i < bucketSize; i++ {
		bucket[i] = g.New()
	}
	mask := (uint64(1) << c) - 1
	j := 0
	var cur uint32
	for cur <= numBits {
		acc.Zero()
		bucket = make([]*PointG2, (1<<c)-1)
		for i := 0; i < len(bucket); i++ {
			bucket[i] = g.New()
		}
		for i := 0; i < len(powers); i++ {
			s0 := powers[i].Uint64()
			index := uint(s0 & mask)
			if index != 0 {
				g.Add(bucket[index-1], bucket[index-1], points[i])
			}
			powers[i] = new(big.Int).Rsh(powers[i], uint(c))
		}
		sum.Zero()
		for i := len(bucket) - 1; i >= 0; i-- {
			g.Add(sum, sum, bucket[i])
			g.Add(acc, acc, sum)
		}
		windows[j] = g.New()
		windows[j].Set(acc)
		j++
		cur += c
	}
	acc.Zero()
	for i := len(windows) - 1; i >= 0; i-- {
		for j := uint32(0); j < c; j++ {
			g.Double(acc, acc)
		}
		g.Add(acc, acc, windows[i])
	}
	return r.Set(acc), nil
}





func (g *G2) MapToCurve(in []byte) (*PointG2, error) {
	fp2 := g.f
	u, err := fp2.fromBytes(in)
	if err != nil {
		return nil, err
	}
	x, y := swuMapG2(fp2, u)
	isogenyMapG2(fp2, x, y)
	z := new(fe2).one()
	q := &PointG2{*x, *y, *z}
	g.ClearCofactor(q)
	return g.Affine(q), nil
}
