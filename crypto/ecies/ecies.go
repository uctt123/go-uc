




























package ecies

import (
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"math/big"
)

var (
	ErrImport                     = fmt.Errorf("ecies: failed to import key")
	ErrInvalidCurve               = fmt.Errorf("ecies: invalid elliptic curve")
	ErrInvalidPublicKey           = fmt.Errorf("ecies: invalid public key")
	ErrSharedKeyIsPointAtInfinity = fmt.Errorf("ecies: shared key is point at infinity")
	ErrSharedKeyTooBig            = fmt.Errorf("ecies: shared key params are too big")
)


type PublicKey struct {
	X *big.Int
	Y *big.Int
	elliptic.Curve
	Params *ECIESParams
}


func (pub *PublicKey) ExportECDSA() *ecdsa.PublicKey {
	return &ecdsa.PublicKey{Curve: pub.Curve, X: pub.X, Y: pub.Y}
}


func ImportECDSAPublic(pub *ecdsa.PublicKey) *PublicKey {
	return &PublicKey{
		X:      pub.X,
		Y:      pub.Y,
		Curve:  pub.Curve,
		Params: ParamsFromCurve(pub.Curve),
	}
}


type PrivateKey struct {
	PublicKey
	D *big.Int
}


func (prv *PrivateKey) ExportECDSA() *ecdsa.PrivateKey {
	pub := &prv.PublicKey
	pubECDSA := pub.ExportECDSA()
	return &ecdsa.PrivateKey{PublicKey: *pubECDSA, D: prv.D}
}


func ImportECDSA(prv *ecdsa.PrivateKey) *PrivateKey {
	pub := ImportECDSAPublic(&prv.PublicKey)
	return &PrivateKey{*pub, prv.D}
}



func GenerateKey(rand io.Reader, curve elliptic.Curve, params *ECIESParams) (prv *PrivateKey, err error) {
	pb, x, y, err := elliptic.GenerateKey(curve, rand)
	if err != nil {
		return
	}
	prv = new(PrivateKey)
	prv.PublicKey.X = x
	prv.PublicKey.Y = y
	prv.PublicKey.Curve = curve
	prv.D = new(big.Int).SetBytes(pb)
	if params == nil {
		params = ParamsFromCurve(curve)
	}
	prv.PublicKey.Params = params
	return
}



func MaxSharedKeyLength(pub *PublicKey) int {
	return (pub.Curve.Params().BitSize + 7) / 8
}


func (prv *PrivateKey) GenerateShared(pub *PublicKey, skLen, macLen int) (sk []byte, err error) {
	if prv.PublicKey.Curve != pub.Curve {
		return nil, ErrInvalidCurve
	}
	if skLen+macLen > MaxSharedKeyLength(pub) {
		return nil, ErrSharedKeyTooBig
	}

	x, _ := pub.Curve.ScalarMult(pub.X, pub.Y, prv.D.Bytes())
	if x == nil {
		return nil, ErrSharedKeyIsPointAtInfinity
	}

	sk = make([]byte, skLen+macLen)
	skBytes := x.Bytes()
	copy(sk[len(sk)-len(skBytes):], skBytes)
	return sk, nil
}

var (
	ErrSharedTooLong  = fmt.Errorf("ecies: shared secret is too long")
	ErrInvalidMessage = fmt.Errorf("ecies: invalid message")
)


func concatKDF(hash hash.Hash, z, s1 []byte, kdLen int) []byte {
	counterBytes := make([]byte, 4)
	k := make([]byte, 0, roundup(kdLen, hash.Size()))
	for counter := uint32(1); len(k) < kdLen; counter++ {
		binary.BigEndian.PutUint32(counterBytes, counter)
		hash.Reset()
		hash.Write(counterBytes)
		hash.Write(z)
		hash.Write(s1)
		k = hash.Sum(k)
	}
	return k[:kdLen]
}


func roundup(size, blocksize int) int {
	return size + blocksize - (size % blocksize)
}


func deriveKeys(hash hash.Hash, z, s1 []byte, keyLen int) (Ke, Km []byte) {
	K := concatKDF(hash, z, s1, 2*keyLen)
	Ke = K[:keyLen]
	Km = K[keyLen:]
	hash.Reset()
	hash.Write(Km)
	Km = hash.Sum(Km[:0])
	return Ke, Km
}



func messageTag(hash func() hash.Hash, km, msg, shared []byte) []byte {
	mac := hmac.New(hash, km)
	mac.Write(msg)
	mac.Write(shared)
	tag := mac.Sum(nil)
	return tag
}


func generateIV(params *ECIESParams, rand io.Reader) (iv []byte, err error) {
	iv = make([]byte, params.BlockSize)
	_, err = io.ReadFull(rand, iv)
	return
}


func symEncrypt(rand io.Reader, params *ECIESParams, key, m []byte) (ct []byte, err error) {
	c, err := params.Cipher(key)
	if err != nil {
		return
	}

	iv, err := generateIV(params, rand)
	if err != nil {
		return
	}
	ctr := cipher.NewCTR(c, iv)

	ct = make([]byte, len(m)+params.BlockSize)
	copy(ct, iv)
	ctr.XORKeyStream(ct[params.BlockSize:], m)
	return
}



func symDecrypt(params *ECIESParams, key, ct []byte) (m []byte, err error) {
	c, err := params.Cipher(key)
	if err != nil {
		return
	}

	ctr := cipher.NewCTR(c, ct[:params.BlockSize])

	m = make([]byte, len(ct)-params.BlockSize)
	ctr.XORKeyStream(m, ct[params.BlockSize:])
	return
}






func Encrypt(rand io.Reader, pub *PublicKey, m, s1, s2 []byte) (ct []byte, err error) {
	params, err := pubkeyParams(pub)
	if err != nil {
		return nil, err
	}

	R, err := GenerateKey(rand, pub.Curve, params)
	if err != nil {
		return nil, err
	}

	z, err := R.GenerateShared(pub, params.KeyLen, params.KeyLen)
	if err != nil {
		return nil, err
	}

	hash := params.Hash()
	Ke, Km := deriveKeys(hash, z, s1, params.KeyLen)

	em, err := symEncrypt(rand, params, Ke, m)
	if err != nil || len(em) <= params.BlockSize {
		return nil, err
	}

	d := messageTag(params.Hash, Km, em, s2)

	Rb := elliptic.Marshal(pub.Curve, R.PublicKey.X, R.PublicKey.Y)
	ct = make([]byte, len(Rb)+len(em)+len(d))
	copy(ct, Rb)
	copy(ct[len(Rb):], em)
	copy(ct[len(Rb)+len(em):], d)
	return ct, nil
}


func (prv *PrivateKey) Decrypt(c, s1, s2 []byte) (m []byte, err error) {
	if len(c) == 0 {
		return nil, ErrInvalidMessage
	}
	params, err := pubkeyParams(&prv.PublicKey)
	if err != nil {
		return nil, err
	}

	hash := params.Hash()

	var (
		rLen   int
		hLen   int = hash.Size()
		mStart int
		mEnd   int
	)

	switch c[0] {
	case 2, 3, 4:
		rLen = (prv.PublicKey.Curve.Params().BitSize + 7) / 4
		if len(c) < (rLen + hLen + 1) {
			return nil, ErrInvalidMessage
		}
	default:
		return nil, ErrInvalidPublicKey
	}

	mStart = rLen
	mEnd = len(c) - hLen

	R := new(PublicKey)
	R.Curve = prv.PublicKey.Curve
	R.X, R.Y = elliptic.Unmarshal(R.Curve, c[:rLen])
	if R.X == nil {
		return nil, ErrInvalidPublicKey
	}

	z, err := prv.GenerateShared(R, params.KeyLen, params.KeyLen)
	if err != nil {
		return nil, err
	}
	Ke, Km := deriveKeys(hash, z, s1, params.KeyLen)

	d := messageTag(params.Hash, Km, c[mStart:mEnd], s2)
	if subtle.ConstantTimeCompare(c[mEnd:], d) != 1 {
		return nil, ErrInvalidMessage
	}

	return symDecrypt(params, Ke, c[mStart:mEnd])
}
