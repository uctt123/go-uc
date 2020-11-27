

















package bls12381

import (
	"golang.org/x/sys/cpu"
)

func init() {
	if !enableADX || !cpu.X86.HasADX || !cpu.X86.HasBMI2 {
		mul = mulNoADX
	}
}


var mul func(c, a, b *fe) = mulADX

func square(c, a *fe) {
	mul(c, a, a)
}

func neg(c, a *fe) {
	if a.isZero() {
		c.set(a)
	} else {
		_neg(c, a)
	}
}


func add(c, a, b *fe)


func addAssign(a, b *fe)


func ladd(c, a, b *fe)


func laddAssign(a, b *fe)


func double(c, a *fe)


func doubleAssign(a *fe)


func ldouble(c, a *fe)


func sub(c, a, b *fe)


func subAssign(a, b *fe)


func lsubAssign(a, b *fe)


func _neg(c, a *fe)


func mulNoADX(c, a, b *fe)


func mulADX(c, a, b *fe)
