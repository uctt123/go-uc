















package bls12381

import (
	"errors"
	"math/big"
)


type E = fe12


type GT struct {
	fp12 *fp12
}

func (e *E) Set(e2 *E) *E {
	return e.set(e2)
}


func (e *E) One() *E {
	e = new(fe12).one()
	return e
}


func (e *E) IsOne() bool {
	return e.isOne()
}


func (g *E) Equal(g2 *E) bool {
	return g.equal(g2)
}


func NewGT() *GT {
	fp12 := newFp12(nil)
	return &GT{fp12}
}


func (g *GT) Q() *big.Int {
	return new(big.Int).Set(q)
}



func (g *GT) FromBytes(in []byte) (*E, error) {
	e, err := g.fp12.fromBytes(in)
	if err != nil {
		return nil, err
	}
	if !g.IsValid(e) {
		return e, errors.New("invalid element")
	}
	return e, nil
}


func (g *GT) ToBytes(e *E) []byte {
	return g.fp12.toBytes(e)
}


func (g *GT) IsValid(e *E) bool {
	r := g.New()
	g.fp12.exp(r, e, q)
	return r.isOne()
}


func (g *GT) New() *E {
	return new(E).One()
}


func (g *GT) Add(c, a, b *E) {
	g.fp12.add(c, a, b)
}


func (g *GT) Sub(c, a, b *E) {
	g.fp12.sub(c, a, b)
}


func (g *GT) Mul(c, a, b *E) {
	g.fp12.mul(c, a, b)
}


func (g *GT) Square(c, a *E) {
	g.fp12.cyclotomicSquare(c, a)
}


func (g *GT) Exp(c, a *E, s *big.Int) {
	g.fp12.cyclotomicExp(c, a, s)
}


func (g *GT) Inverse(c, a *E) {
	g.fp12.inverse(c, a)
}
