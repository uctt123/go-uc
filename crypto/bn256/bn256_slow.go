






package bn256

import bn256 "github.com/ethereum/go-ethereum/crypto/bn256/google"



type G1 = bn256.G1



type G2 = bn256.G2


func PairingCheck(a []*G1, b []*G2) bool {
	return bn256.PairingCheck(a, b)
}
