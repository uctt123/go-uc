















package v5wire

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"fmt"
	"hash"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"golang.org/x/crypto/hkdf"
)

const (
	
	aesKeySize   = 16
	gcmNonceSize = 12
)


type Nonce [gcmNonceSize]byte


func EncodePubkey(key *ecdsa.PublicKey) []byte {
	switch key.Curve {
	case crypto.S256():
		return crypto.CompressPubkey(key)
	default:
		panic("unsupported curve " + key.Curve.Params().Name + " in EncodePubkey")
	}
}


func DecodePubkey(curve elliptic.Curve, e []byte) (*ecdsa.PublicKey, error) {
	switch curve {
	case crypto.S256():
		if len(e) != 33 {
			return nil, errors.New("wrong size public key data")
		}
		return crypto.DecompressPubkey(e)
	default:
		return nil, fmt.Errorf("unsupported curve %s in DecodePubkey", curve.Params().Name)
	}
}


func idNonceHash(h hash.Hash, challenge, ephkey []byte, destID enode.ID) []byte {
	h.Reset()
	h.Write([]byte("discovery v5 identity proof"))
	h.Write(challenge)
	h.Write(ephkey)
	h.Write(destID[:])
	return h.Sum(nil)
}


func makeIDSignature(hash hash.Hash, key *ecdsa.PrivateKey, challenge, ephkey []byte, destID enode.ID) ([]byte, error) {
	input := idNonceHash(hash, challenge, ephkey, destID)
	switch key.Curve {
	case crypto.S256():
		idsig, err := crypto.Sign(input, key)
		if err != nil {
			return nil, err
		}
		return idsig[:len(idsig)-1], nil 
	default:
		return nil, fmt.Errorf("unsupported curve %s", key.Curve.Params().Name)
	}
}


type s256raw []byte

func (s256raw) ENRKey() string { return "secp256k1" }


func verifyIDSignature(hash hash.Hash, sig []byte, n *enode.Node, challenge, ephkey []byte, destID enode.ID) error {
	switch idscheme := n.Record().IdentityScheme(); idscheme {
	case "v4":
		var pubkey s256raw
		if n.Load(&pubkey) != nil {
			return errors.New("no secp256k1 public key in record")
		}
		input := idNonceHash(hash, challenge, ephkey, destID)
		if !crypto.VerifySignature(pubkey, input, sig) {
			return errInvalidNonceSig
		}
		return nil
	default:
		return fmt.Errorf("can't verify ID nonce signature against scheme %q", idscheme)
	}
}

type hashFn func() hash.Hash


func deriveKeys(hash hashFn, priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey, n1, n2 enode.ID, challenge []byte) *session {
	const text = "discovery v5 key agreement"
	var info = make([]byte, 0, len(text)+len(n1)+len(n2))
	info = append(info, text...)
	info = append(info, n1[:]...)
	info = append(info, n2[:]...)

	eph := ecdh(priv, pub)
	if eph == nil {
		return nil
	}
	kdf := hkdf.New(hash, eph, challenge, info)
	sec := session{writeKey: make([]byte, aesKeySize), readKey: make([]byte, aesKeySize)}
	kdf.Read(sec.writeKey)
	kdf.Read(sec.readKey)
	for i := range eph {
		eph[i] = 0
	}
	return &sec
}


func ecdh(privkey *ecdsa.PrivateKey, pubkey *ecdsa.PublicKey) []byte {
	secX, secY := pubkey.ScalarMult(pubkey.X, pubkey.Y, privkey.D.Bytes())
	if secX == nil {
		return nil
	}
	sec := make([]byte, 33)
	sec[0] = 0x02 | byte(secY.Bit(0))
	math.ReadBits(secX, sec[1:])
	return sec
}




func encryptGCM(dest, key, nonce, plaintext, authData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Errorf("can't create block cipher: %v", err))
	}
	aesgcm, err := cipher.NewGCMWithNonceSize(block, gcmNonceSize)
	if err != nil {
		panic(fmt.Errorf("can't create GCM: %v", err))
	}
	return aesgcm.Seal(dest, nonce, plaintext, authData), nil
}


func decryptGCM(key, nonce, ct, authData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("can't create block cipher: %v", err)
	}
	if len(nonce) != gcmNonceSize {
		return nil, fmt.Errorf("invalid GCM nonce size: %d", len(nonce))
	}
	aesgcm, err := cipher.NewGCMWithNonceSize(block, gcmNonceSize)
	if err != nil {
		return nil, fmt.Errorf("can't create GCM: %v", err)
	}
	pt := make([]byte, 0, len(ct))
	return aesgcm.Open(pt, nonce, ct, authData)
}
