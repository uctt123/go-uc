






package bn256

import (
	bn256cf "github.com/ethereum/go-ethereum/crypto/bn256/cloudflare"
)



type G1 = bn256cf.G1



type G2 = bn256cf.G2


func PairingCheck(a []*G1, b []*G2) bool {
	return bn256cf.PairingCheck(a, b)
}
