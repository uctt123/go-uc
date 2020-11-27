
















package rlpx

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	mrand "math/rand"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/golang/snappy"
	"golang.org/x/crypto/sha3"
)







type Conn struct {
	dialDest  *ecdsa.PublicKey
	conn      net.Conn
	handshake *handshakeState
	snappy    bool
}

type handshakeState struct {
	enc cipher.Stream
	dec cipher.Stream

	macCipher  cipher.Block
	egressMAC  hash.Hash
	ingressMAC hash.Hash
}



func NewConn(conn net.Conn, dialDest *ecdsa.PublicKey) *Conn {
	return &Conn{
		dialDest: dialDest,
		conn:     conn,
	}
}




func (c *Conn) SetSnappy(snappy bool) {
	c.snappy = snappy
}


func (c *Conn) SetReadDeadline(time time.Time) error {
	return c.conn.SetReadDeadline(time)
}


func (c *Conn) SetWriteDeadline(time time.Time) error {
	return c.conn.SetWriteDeadline(time)
}


func (c *Conn) SetDeadline(time time.Time) error {
	return c.conn.SetDeadline(time)
}


func (c *Conn) Read() (code uint64, data []byte, wireSize int, err error) {
	if c.handshake == nil {
		panic("can't ReadMsg before handshake")
	}

	frame, err := c.handshake.readFrame(c.conn)
	if err != nil {
		return 0, nil, 0, err
	}
	code, data, err = rlp.SplitUint64(frame)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("invalid message code: %v", err)
	}
	wireSize = len(data)

	
	if c.snappy {
		var actualSize int
		actualSize, err = snappy.DecodedLen(data)
		if err != nil {
			return code, nil, 0, err
		}
		if actualSize > maxUint24 {
			return code, nil, 0, errPlainMessageTooLarge
		}
		data, err = snappy.Decode(nil, data)
	}
	return code, data, wireSize, err
}

func (h *handshakeState) readFrame(conn io.Reader) ([]byte, error) {
	
	headbuf := make([]byte, 32)
	if _, err := io.ReadFull(conn, headbuf); err != nil {
		return nil, err
	}

	
	shouldMAC := updateMAC(h.ingressMAC, h.macCipher, headbuf[:16])
	if !hmac.Equal(shouldMAC, headbuf[16:]) {
		return nil, errors.New("bad header MAC")
	}
	h.dec.XORKeyStream(headbuf[:16], headbuf[:16]) 
	fsize := readInt24(headbuf)
	

	
	var rsize = fsize 
	if padding := fsize % 16; padding > 0 {
		rsize += 16 - padding
	}
	framebuf := make([]byte, rsize)
	if _, err := io.ReadFull(conn, framebuf); err != nil {
		return nil, err
	}

	
	h.ingressMAC.Write(framebuf)
	fmacseed := h.ingressMAC.Sum(nil)
	if _, err := io.ReadFull(conn, headbuf[:16]); err != nil {
		return nil, err
	}
	shouldMAC = updateMAC(h.ingressMAC, h.macCipher, fmacseed)
	if !hmac.Equal(shouldMAC, headbuf[:16]) {
		return nil, errors.New("bad frame MAC")
	}

	
	h.dec.XORKeyStream(framebuf, framebuf)
	return framebuf[:fsize], nil
}





func (c *Conn) Write(code uint64, data []byte) (uint32, error) {
	if c.handshake == nil {
		panic("can't WriteMsg before handshake")
	}
	if len(data) > maxUint24 {
		return 0, errPlainMessageTooLarge
	}
	if c.snappy {
		data = snappy.Encode(nil, data)
	}

	wireSize := uint32(len(data))
	err := c.handshake.writeFrame(c.conn, code, data)
	return wireSize, err
}

func (h *handshakeState) writeFrame(conn io.Writer, code uint64, data []byte) error {
	ptype, _ := rlp.EncodeToBytes(code)

	
	headbuf := make([]byte, 32)
	fsize := len(ptype) + len(data)
	if fsize > maxUint24 {
		return errPlainMessageTooLarge
	}
	putInt24(uint32(fsize), headbuf)
	copy(headbuf[3:], zeroHeader)
	h.enc.XORKeyStream(headbuf[:16], headbuf[:16]) 

	
	copy(headbuf[16:], updateMAC(h.egressMAC, h.macCipher, headbuf[:16]))
	if _, err := conn.Write(headbuf); err != nil {
		return err
	}

	
	
	tee := cipher.StreamWriter{S: h.enc, W: io.MultiWriter(conn, h.egressMAC)}
	if _, err := tee.Write(ptype); err != nil {
		return err
	}
	if _, err := tee.Write(data); err != nil {
		return err
	}
	if padding := fsize % 16; padding > 0 {
		if _, err := tee.Write(zero16[:16-padding]); err != nil {
			return err
		}
	}

	
	
	fmacseed := h.egressMAC.Sum(nil)
	mac := updateMAC(h.egressMAC, h.macCipher, fmacseed)
	_, err := conn.Write(mac)
	return err
}

func readInt24(b []byte) uint32 {
	return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16
}

func putInt24(v uint32, b []byte) {
	b[0] = byte(v >> 16)
	b[1] = byte(v >> 8)
	b[2] = byte(v)
}



func updateMAC(mac hash.Hash, block cipher.Block, seed []byte) []byte {
	aesbuf := make([]byte, aes.BlockSize)
	block.Encrypt(aesbuf, mac.Sum(nil))
	for i := range aesbuf {
		aesbuf[i] ^= seed[i]
	}
	mac.Write(aesbuf)
	return mac.Sum(nil)[:16]
}



func (c *Conn) Handshake(prv *ecdsa.PrivateKey) (*ecdsa.PublicKey, error) {
	var (
		sec Secrets
		err error
	)
	if c.dialDest != nil {
		sec, err = initiatorEncHandshake(c.conn, prv, c.dialDest)
	} else {
		sec, err = receiverEncHandshake(c.conn, prv)
	}
	if err != nil {
		return nil, err
	}
	c.InitWithSecrets(sec)
	return sec.remote, err
}



func (c *Conn) InitWithSecrets(sec Secrets) {
	if c.handshake != nil {
		panic("can't handshake twice")
	}
	macc, err := aes.NewCipher(sec.MAC)
	if err != nil {
		panic("invalid MAC secret: " + err.Error())
	}
	encc, err := aes.NewCipher(sec.AES)
	if err != nil {
		panic("invalid AES secret: " + err.Error())
	}
	
	
	iv := make([]byte, encc.BlockSize())
	c.handshake = &handshakeState{
		enc:        cipher.NewCTR(encc, iv),
		dec:        cipher.NewCTR(encc, iv),
		macCipher:  macc,
		egressMAC:  sec.EgressMAC,
		ingressMAC: sec.IngressMAC,
	}
}


func (c *Conn) Close() error {
	return c.conn.Close()
}


const (
	maxUint24 = int(^uint32(0) >> 8)

	sskLen = 16                     
	sigLen = crypto.SignatureLength 
	pubLen = 64                     
	shaLen = 32                     

	authMsgLen  = sigLen + shaLen + pubLen + shaLen + 1
	authRespLen = pubLen + shaLen + 1

	eciesOverhead = 65 /* pubkey */ + 16 /* IV */ + 32 /* MAC */

	encAuthMsgLen  = authMsgLen + eciesOverhead  
	encAuthRespLen = authRespLen + eciesOverhead 
)

var (
	
	
	zeroHeader = []byte{0xC2, 0x80, 0x80}
	
	zero16 = make([]byte, 16)

	
	
	errPlainMessageTooLarge = errors.New("message length >= 16MB")
)


type Secrets struct {
	AES, MAC              []byte
	EgressMAC, IngressMAC hash.Hash
	remote                *ecdsa.PublicKey
}


type encHandshake struct {
	initiator            bool
	remote               *ecies.PublicKey  
	initNonce, respNonce []byte            
	randomPrivKey        *ecies.PrivateKey 
	remoteRandomPub      *ecies.PublicKey  
}


type authMsgV4 struct {
	gotPlain bool 

	Signature       [sigLen]byte
	InitiatorPubkey [pubLen]byte
	Nonce           [shaLen]byte
	Version         uint

	
	Rest []rlp.RawValue `rlp:"tail"`
}


type authRespV4 struct {
	RandomPubkey [pubLen]byte
	Nonce        [shaLen]byte
	Version      uint

	
	Rest []rlp.RawValue `rlp:"tail"`
}





func receiverEncHandshake(conn io.ReadWriter, prv *ecdsa.PrivateKey) (s Secrets, err error) {
	authMsg := new(authMsgV4)
	authPacket, err := readHandshakeMsg(authMsg, encAuthMsgLen, prv, conn)
	if err != nil {
		return s, err
	}
	h := new(encHandshake)
	if err := h.handleAuthMsg(authMsg, prv); err != nil {
		return s, err
	}

	authRespMsg, err := h.makeAuthResp()
	if err != nil {
		return s, err
	}
	var authRespPacket []byte
	if authMsg.gotPlain {
		authRespPacket, err = authRespMsg.sealPlain(h)
	} else {
		authRespPacket, err = sealEIP8(authRespMsg, h)
	}
	if err != nil {
		return s, err
	}
	if _, err = conn.Write(authRespPacket); err != nil {
		return s, err
	}
	return h.secrets(authPacket, authRespPacket)
}

func (h *encHandshake) handleAuthMsg(msg *authMsgV4, prv *ecdsa.PrivateKey) error {
	
	rpub, err := importPublicKey(msg.InitiatorPubkey[:])
	if err != nil {
		return err
	}
	h.initNonce = msg.Nonce[:]
	h.remote = rpub

	
	
	if h.randomPrivKey == nil {
		h.randomPrivKey, err = ecies.GenerateKey(rand.Reader, crypto.S256(), nil)
		if err != nil {
			return err
		}
	}

	
	token, err := h.staticSharedSecret(prv)
	if err != nil {
		return err
	}
	signedMsg := xor(token, h.initNonce)
	remoteRandomPub, err := crypto.Ecrecover(signedMsg, msg.Signature[:])
	if err != nil {
		return err
	}
	h.remoteRandomPub, _ = importPublicKey(remoteRandomPub)
	return nil
}



func (h *encHandshake) secrets(auth, authResp []byte) (Secrets, error) {
	ecdheSecret, err := h.randomPrivKey.GenerateShared(h.remoteRandomPub, sskLen, sskLen)
	if err != nil {
		return Secrets{}, err
	}

	
	sharedSecret := crypto.Keccak256(ecdheSecret, crypto.Keccak256(h.respNonce, h.initNonce))
	aesSecret := crypto.Keccak256(ecdheSecret, sharedSecret)
	s := Secrets{
		remote: h.remote.ExportECDSA(),
		AES:    aesSecret,
		MAC:    crypto.Keccak256(ecdheSecret, aesSecret),
	}

	
	mac1 := sha3.NewLegacyKeccak256()
	mac1.Write(xor(s.MAC, h.respNonce))
	mac1.Write(auth)
	mac2 := sha3.NewLegacyKeccak256()
	mac2.Write(xor(s.MAC, h.initNonce))
	mac2.Write(authResp)
	if h.initiator {
		s.EgressMAC, s.IngressMAC = mac1, mac2
	} else {
		s.EgressMAC, s.IngressMAC = mac2, mac1
	}

	return s, nil
}



func (h *encHandshake) staticSharedSecret(prv *ecdsa.PrivateKey) ([]byte, error) {
	return ecies.ImportECDSA(prv).GenerateShared(h.remote, sskLen, sskLen)
}





func initiatorEncHandshake(conn io.ReadWriter, prv *ecdsa.PrivateKey, remote *ecdsa.PublicKey) (s Secrets, err error) {
	h := &encHandshake{initiator: true, remote: ecies.ImportECDSAPublic(remote)}
	authMsg, err := h.makeAuthMsg(prv)
	if err != nil {
		return s, err
	}
	authPacket, err := sealEIP8(authMsg, h)
	if err != nil {
		return s, err
	}

	if _, err = conn.Write(authPacket); err != nil {
		return s, err
	}

	authRespMsg := new(authRespV4)
	authRespPacket, err := readHandshakeMsg(authRespMsg, encAuthRespLen, prv, conn)
	if err != nil {
		return s, err
	}
	if err := h.handleAuthResp(authRespMsg); err != nil {
		return s, err
	}
	return h.secrets(authPacket, authRespPacket)
}


func (h *encHandshake) makeAuthMsg(prv *ecdsa.PrivateKey) (*authMsgV4, error) {
	
	h.initNonce = make([]byte, shaLen)
	_, err := rand.Read(h.initNonce)
	if err != nil {
		return nil, err
	}
	
	h.randomPrivKey, err = ecies.GenerateKey(rand.Reader, crypto.S256(), nil)
	if err != nil {
		return nil, err
	}

	
	token, err := h.staticSharedSecret(prv)
	if err != nil {
		return nil, err
	}
	signed := xor(token, h.initNonce)
	signature, err := crypto.Sign(signed, h.randomPrivKey.ExportECDSA())
	if err != nil {
		return nil, err
	}

	msg := new(authMsgV4)
	copy(msg.Signature[:], signature)
	copy(msg.InitiatorPubkey[:], crypto.FromECDSAPub(&prv.PublicKey)[1:])
	copy(msg.Nonce[:], h.initNonce)
	msg.Version = 4
	return msg, nil
}

func (h *encHandshake) handleAuthResp(msg *authRespV4) (err error) {
	h.respNonce = msg.Nonce[:]
	h.remoteRandomPub, err = importPublicKey(msg.RandomPubkey[:])
	return err
}

func (h *encHandshake) makeAuthResp() (msg *authRespV4, err error) {
	
	h.respNonce = make([]byte, shaLen)
	if _, err = rand.Read(h.respNonce); err != nil {
		return nil, err
	}

	msg = new(authRespV4)
	copy(msg.Nonce[:], h.respNonce)
	copy(msg.RandomPubkey[:], exportPubkey(&h.randomPrivKey.PublicKey))
	msg.Version = 4
	return msg, nil
}

func (msg *authMsgV4) decodePlain(input []byte) {
	n := copy(msg.Signature[:], input)
	n += shaLen 
	n += copy(msg.InitiatorPubkey[:], input[n:])
	copy(msg.Nonce[:], input[n:])
	msg.Version = 4
	msg.gotPlain = true
}

func (msg *authRespV4) sealPlain(hs *encHandshake) ([]byte, error) {
	buf := make([]byte, authRespLen)
	n := copy(buf, msg.RandomPubkey[:])
	copy(buf[n:], msg.Nonce[:])
	return ecies.Encrypt(rand.Reader, hs.remote, buf, nil, nil)
}

func (msg *authRespV4) decodePlain(input []byte) {
	n := copy(msg.RandomPubkey[:], input)
	copy(msg.Nonce[:], input[n:])
	msg.Version = 4
}

var padSpace = make([]byte, 300)

func sealEIP8(msg interface{}, h *encHandshake) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := rlp.Encode(buf, msg); err != nil {
		return nil, err
	}
	
	
	pad := padSpace[:mrand.Intn(len(padSpace)-100)+100]
	buf.Write(pad)
	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(buf.Len()+eciesOverhead))

	enc, err := ecies.Encrypt(rand.Reader, h.remote, buf.Bytes(), nil, prefix)
	return append(prefix, enc...), err
}

type plainDecoder interface {
	decodePlain([]byte)
}

func readHandshakeMsg(msg plainDecoder, plainSize int, prv *ecdsa.PrivateKey, r io.Reader) ([]byte, error) {
	buf := make([]byte, plainSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return buf, err
	}
	
	key := ecies.ImportECDSA(prv)
	if dec, err := key.Decrypt(buf, nil, nil); err == nil {
		msg.decodePlain(dec)
		return buf, nil
	}
	
	prefix := buf[:2]
	size := binary.BigEndian.Uint16(prefix)
	if size < uint16(plainSize) {
		return buf, fmt.Errorf("size underflow, need at least %d bytes", plainSize)
	}
	buf = append(buf, make([]byte, size-uint16(plainSize)+2)...)
	if _, err := io.ReadFull(r, buf[plainSize:]); err != nil {
		return buf, err
	}
	dec, err := key.Decrypt(buf[2:], nil, prefix)
	if err != nil {
		return buf, err
	}
	
	
	s := rlp.NewStream(bytes.NewReader(dec), 0)
	return buf, s.Decode(msg)
}


func importPublicKey(pubKey []byte) (*ecies.PublicKey, error) {
	var pubKey65 []byte
	switch len(pubKey) {
	case 64:
		
		pubKey65 = append([]byte{0x04}, pubKey...)
	case 65:
		pubKey65 = pubKey
	default:
		return nil, fmt.Errorf("invalid public key length %v (expect 64/65)", len(pubKey))
	}
	
	pub, err := crypto.UnmarshalPubkey(pubKey65)
	if err != nil {
		return nil, err
	}
	return ecies.ImportECDSAPublic(pub), nil
}

func exportPubkey(pub *ecies.PublicKey) []byte {
	if pub == nil {
		panic("nil pubkey")
	}
	return elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:]
}

func xor(one, other []byte) (xor []byte) {
	xor = make([]byte, len(one))
	for i := 0; i < len(one); i++ {
		xor[i] = one[i] ^ other[i]
	}
	return xor
}
