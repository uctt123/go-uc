

package bn256




import (
	"golang.org/x/sys/cpu"
)


var hasBMI2 = cpu.X86.HasBMI2


func gfpNeg(c, a *gfP)


func gfpAdd(c, a, b *gfP)


func gfpSub(c, a, b *gfP)


func gfpMul(c, a, b *gfP)
