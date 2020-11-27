















package v5wire

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
)





type Header struct {
	IV [sizeofMaskingIV]byte
	StaticHeader
	AuthData []byte

	src enode.ID 
}


type StaticHeader struct {
	ProtocolID [6]byte
	Version    uint16
	Flag       byte
	Nonce      Nonce
	AuthSize   uint16
}


type (
	whoareyouAuthData struct {
		IDNonce   [16]byte 
		RecordSeq uint64   
	}

	handshakeAuthData struct {
		h struct {
			SrcID      enode.ID
			SigSize    byte 
			PubkeySize byte 
		}
		
		signature, pubkey, record []byte
	}

	messageAuthData struct {
		SrcID enode.ID
	}
)


const (
	flagMessage = iota
	flagWhoareyou
	flagHandshake
)


const (
	version         = 1
	minVersion      = 1
	sizeofMaskingIV = 16

	minMessageSize      = 48 
	randomPacketMsgSize = 20
)

var protocolID = [6]byte{'d', 'i', 's', 'c', 'v', '5'}


var (
	errTooShort            = errors.New("packet too short")
	errInvalidHeader       = errors.New("invalid packet header")
	errInvalidFlag         = errors.New("invalid flag value in header")
	errMinVersion          = errors.New("version of packet header below minimum")
	errMsgTooShort         = errors.New("message/handshake packet below minimum size")
	errAuthSize            = errors.New("declared auth size is beyond packet length")
	errUnexpectedHandshake = errors.New("unexpected auth response, not in handshake")
	errInvalidAuthKey      = errors.New("invalid ephemeral pubkey")
	errNoRecord            = errors.New("expected ENR in handshake but none sent")
	errInvalidNonceSig     = errors.New("invalid ID nonce signature")
	errMessageTooShort     = errors.New("message contains no data")
	errMessageDecrypt      = errors.New("cannot decrypt message")
)


var (
	ErrInvalidReqID = errors.New("request ID larger than 8 bytes")
)


var (
	sizeofStaticHeader      = binary.Size(StaticHeader{})
	sizeofWhoareyouAuthData = binary.Size(whoareyouAuthData{})
	sizeofHandshakeAuthData = binary.Size(handshakeAuthData{}.h)
	sizeofMessageAuthData   = binary.Size(messageAuthData{})
	sizeofStaticPacketData  = sizeofMaskingIV + sizeofStaticHeader
)



type Codec struct {
	sha256    hash.Hash
	localnode *enode.LocalNode
	privkey   *ecdsa.PrivateKey
	sc        *SessionCache

	
	buf      bytes.Buffer 
	headbuf  bytes.Buffer 
	msgbuf   bytes.Buffer 
	msgctbuf []byte       

	
	reader bytes.Reader
}


func NewCodec(ln *enode.LocalNode, key *ecdsa.PrivateKey, clock mclock.Clock) *Codec {
	c := &Codec{
		sha256:    sha256.New(),
		localnode: ln,
		privkey:   key,
		sc:        NewSessionCache(1024, clock),
	}
	return c
}




func (c *Codec) Encode(id enode.ID, addr string, packet Packet, challenge *Whoareyou) ([]byte, Nonce, error) {
	
	var (
		head    Header
		session *session
		msgData []byte
		err     error
	)
	switch {
	case packet.Kind() == WhoareyouPacket:
		head, err = c.encodeWhoareyou(id, packet.(*Whoareyou))
	case challenge != nil:
		
		head, session, err = c.encodeHandshakeHeader(id, addr, challenge)
	default:
		session = c.sc.session(id, addr)
		if session != nil {
			
			head, err = c.encodeMessageHeader(id, session)
		} else {
			
			head, msgData, err = c.encodeRandom(id)
		}
	}
	if err != nil {
		return nil, Nonce{}, err
	}

	
	if err := c.sc.maskingIVGen(head.IV[:]); err != nil {
		return nil, Nonce{}, fmt.Errorf("can't generate masking IV: %v", err)
	}

	
	c.writeHeaders(&head)

	
	if challenge, ok := packet.(*Whoareyou); ok {
		challenge.ChallengeData = bytesCopy(&c.buf)
		c.sc.storeSentHandshake(id, addr, challenge)
	} else if msgData == nil {
		headerData := c.buf.Bytes()
		msgData, err = c.encryptMessage(session, packet, &head, headerData)
		if err != nil {
			return nil, Nonce{}, err
		}
	}

	enc, err := c.EncodeRaw(id, head, msgData)
	return enc, head.Nonce, err
}


func (c *Codec) EncodeRaw(id enode.ID, head Header, msgdata []byte) ([]byte, error) {
	c.writeHeaders(&head)

	
	masked := c.buf.Bytes()[sizeofMaskingIV:]
	mask := head.mask(id)
	mask.XORKeyStream(masked[:], masked[:])

	
	c.buf.Write(msgdata)
	return c.buf.Bytes(), nil
}

func (c *Codec) writeHeaders(head *Header) {
	c.buf.Reset()
	c.buf.Write(head.IV[:])
	binary.Write(&c.buf, binary.BigEndian, &head.StaticHeader)
	c.buf.Write(head.AuthData)
}


func (c *Codec) makeHeader(toID enode.ID, flag byte, authsizeExtra int) Header {
	var authsize int
	switch flag {
	case flagMessage:
		authsize = sizeofMessageAuthData
	case flagWhoareyou:
		authsize = sizeofWhoareyouAuthData
	case flagHandshake:
		authsize = sizeofHandshakeAuthData
	default:
		panic(fmt.Errorf("BUG: invalid packet header flag %x", flag))
	}
	authsize += authsizeExtra
	if authsize > int(^uint16(0)) {
		panic(fmt.Errorf("BUG: auth size %d overflows uint16", authsize))
	}
	return Header{
		StaticHeader: StaticHeader{
			ProtocolID: protocolID,
			Version:    version,
			Flag:       flag,
			AuthSize:   uint16(authsize),
		},
	}
}


func (c *Codec) encodeRandom(toID enode.ID) (Header, []byte, error) {
	head := c.makeHeader(toID, flagMessage, 0)

	
	auth := messageAuthData{SrcID: c.localnode.ID()}
	if _, err := crand.Read(head.Nonce[:]); err != nil {
		return head, nil, fmt.Errorf("can't get random data: %v", err)
	}
	c.headbuf.Reset()
	binary.Write(&c.headbuf, binary.BigEndian, auth)
	head.AuthData = c.headbuf.Bytes()

	
	c.msgctbuf = append(c.msgctbuf[:0], make([]byte, randomPacketMsgSize)...)
	crand.Read(c.msgctbuf)
	return head, c.msgctbuf, nil
}


func (c *Codec) encodeWhoareyou(toID enode.ID, packet *Whoareyou) (Header, error) {
	
	if packet.RecordSeq > 0 && packet.Node == nil {
		panic("BUG: missing node in whoareyou with non-zero seq")
	}

	
	head := c.makeHeader(toID, flagWhoareyou, 0)
	head.AuthData = bytesCopy(&c.buf)
	head.Nonce = packet.Nonce

	
	auth := &whoareyouAuthData{
		IDNonce:   packet.IDNonce,
		RecordSeq: packet.RecordSeq,
	}
	c.headbuf.Reset()
	binary.Write(&c.headbuf, binary.BigEndian, auth)
	head.AuthData = c.headbuf.Bytes()
	return head, nil
}


func (c *Codec) encodeHandshakeHeader(toID enode.ID, addr string, challenge *Whoareyou) (Header, *session, error) {
	
	if challenge.Node == nil {
		panic("BUG: missing challenge.Node in encode")
	}

	
	auth, session, err := c.makeHandshakeAuth(toID, addr, challenge)
	if err != nil {
		return Header{}, nil, err
	}

	
	nonce, err := c.sc.nextNonce(session)
	if err != nil {
		return Header{}, nil, fmt.Errorf("can't generate nonce: %v", err)
	}

	
	c.sc.storeNewSession(toID, addr, session)

	
	var (
		authsizeExtra = len(auth.pubkey) + len(auth.signature) + len(auth.record)
		head          = c.makeHeader(toID, flagHandshake, authsizeExtra)
	)
	c.headbuf.Reset()
	binary.Write(&c.headbuf, binary.BigEndian, &auth.h)
	c.headbuf.Write(auth.signature)
	c.headbuf.Write(auth.pubkey)
	c.headbuf.Write(auth.record)
	head.AuthData = c.headbuf.Bytes()
	head.Nonce = nonce
	return head, session, err
}


func (c *Codec) makeHandshakeAuth(toID enode.ID, addr string, challenge *Whoareyou) (*handshakeAuthData, *session, error) {
	auth := new(handshakeAuthData)
	auth.h.SrcID = c.localnode.ID()

	
	
	var remotePubkey = new(ecdsa.PublicKey)
	if err := challenge.Node.Load((*enode.Secp256k1)(remotePubkey)); err != nil {
		return nil, nil, fmt.Errorf("can't find secp256k1 key for recipient")
	}
	ephkey, err := c.sc.ephemeralKeyGen()
	if err != nil {
		return nil, nil, fmt.Errorf("can't generate ephemeral key")
	}
	ephpubkey := EncodePubkey(&ephkey.PublicKey)
	auth.pubkey = ephpubkey[:]
	auth.h.PubkeySize = byte(len(auth.pubkey))

	
	cdata := challenge.ChallengeData
	idsig, err := makeIDSignature(c.sha256, c.privkey, cdata, ephpubkey[:], toID)
	if err != nil {
		return nil, nil, fmt.Errorf("can't sign: %v", err)
	}
	auth.signature = idsig
	auth.h.SigSize = byte(len(auth.signature))

	
	ln := c.localnode.Node()
	if challenge.RecordSeq < ln.Seq() {
		auth.record, _ = rlp.EncodeToBytes(ln.Record())
	}

	
	sec := deriveKeys(sha256.New, ephkey, remotePubkey, c.localnode.ID(), challenge.Node.ID(), cdata)
	if sec == nil {
		return nil, nil, fmt.Errorf("key derivation failed")
	}
	return auth, sec, err
}


func (c *Codec) encodeMessageHeader(toID enode.ID, s *session) (Header, error) {
	head := c.makeHeader(toID, flagMessage, 0)

	
	nonce, err := c.sc.nextNonce(s)
	if err != nil {
		return Header{}, fmt.Errorf("can't generate nonce: %v", err)
	}
	auth := messageAuthData{SrcID: c.localnode.ID()}
	c.buf.Reset()
	binary.Write(&c.buf, binary.BigEndian, &auth)
	head.AuthData = bytesCopy(&c.buf)
	head.Nonce = nonce
	return head, err
}

func (c *Codec) encryptMessage(s *session, p Packet, head *Header, headerData []byte) ([]byte, error) {
	
	c.msgbuf.Reset()
	c.msgbuf.WriteByte(p.Kind())
	if err := rlp.Encode(&c.msgbuf, p); err != nil {
		return nil, err
	}
	messagePT := c.msgbuf.Bytes()

	
	messageCT, err := encryptGCM(c.msgctbuf[:0], s.writeKey, head.Nonce[:], messagePT, headerData)
	if err == nil {
		c.msgctbuf = messageCT
	}
	return messageCT, err
}


func (c *Codec) Decode(input []byte, addr string) (src enode.ID, n *enode.Node, p Packet, err error) {
	
	if len(input) < sizeofStaticPacketData {
		return enode.ID{}, nil, nil, errTooShort
	}
	var head Header
	copy(head.IV[:], input[:sizeofMaskingIV])
	mask := head.mask(c.localnode.ID())
	staticHeader := input[sizeofMaskingIV:sizeofStaticPacketData]
	mask.XORKeyStream(staticHeader, staticHeader)

	
	c.reader.Reset(staticHeader)
	binary.Read(&c.reader, binary.BigEndian, &head.StaticHeader)
	remainingInput := len(input) - sizeofStaticPacketData
	if err := head.checkValid(remainingInput); err != nil {
		return enode.ID{}, nil, nil, err
	}

	
	authDataEnd := sizeofStaticPacketData + int(head.AuthSize)
	authData := input[sizeofStaticPacketData:authDataEnd]
	mask.XORKeyStream(authData, authData)
	head.AuthData = authData

	
	
	c.sc.handshakeGC()

	
	headerData := input[:authDataEnd]
	msgData := input[authDataEnd:]
	switch head.Flag {
	case flagWhoareyou:
		p, err = c.decodeWhoareyou(&head, headerData)
	case flagHandshake:
		n, p, err = c.decodeHandshakeMessage(addr, &head, headerData, msgData)
	case flagMessage:
		p, err = c.decodeMessage(addr, &head, headerData, msgData)
	default:
		err = errInvalidFlag
	}
	return head.src, n, p, err
}


func (c *Codec) decodeWhoareyou(head *Header, headerData []byte) (Packet, error) {
	if len(head.AuthData) != sizeofWhoareyouAuthData {
		return nil, fmt.Errorf("invalid auth size %d for WHOAREYOU", len(head.AuthData))
	}
	var auth whoareyouAuthData
	c.reader.Reset(head.AuthData)
	binary.Read(&c.reader, binary.BigEndian, &auth)
	p := &Whoareyou{
		Nonce:         head.Nonce,
		IDNonce:       auth.IDNonce,
		RecordSeq:     auth.RecordSeq,
		ChallengeData: make([]byte, len(headerData)),
	}
	copy(p.ChallengeData, headerData)
	return p, nil
}

func (c *Codec) decodeHandshakeMessage(fromAddr string, head *Header, headerData, msgData []byte) (n *enode.Node, p Packet, err error) {
	node, auth, session, err := c.decodeHandshake(fromAddr, head)
	if err != nil {
		c.sc.deleteHandshake(auth.h.SrcID, fromAddr)
		return nil, nil, err
	}

	
	msg, err := c.decryptMessage(msgData, head.Nonce[:], headerData, session.readKey)
	if err != nil {
		c.sc.deleteHandshake(auth.h.SrcID, fromAddr)
		return node, msg, err
	}

	
	c.sc.storeNewSession(auth.h.SrcID, fromAddr, session)
	c.sc.deleteHandshake(auth.h.SrcID, fromAddr)
	return node, msg, nil
}

func (c *Codec) decodeHandshake(fromAddr string, head *Header) (n *enode.Node, auth handshakeAuthData, s *session, err error) {
	if auth, err = c.decodeHandshakeAuthData(head); err != nil {
		return nil, auth, nil, err
	}

	
	challenge := c.sc.getHandshake(auth.h.SrcID, fromAddr)
	if challenge == nil {
		return nil, auth, nil, errUnexpectedHandshake
	}
	
	n, err = c.decodeHandshakeRecord(challenge.Node, auth.h.SrcID, auth.record)
	if err != nil {
		return nil, auth, nil, err
	}
	
	sig := auth.signature
	cdata := challenge.ChallengeData
	err = verifyIDSignature(c.sha256, sig, n, cdata, auth.pubkey, c.localnode.ID())
	if err != nil {
		return nil, auth, nil, err
	}
	
	ephkey, err := DecodePubkey(c.privkey.Curve, auth.pubkey)
	if err != nil {
		return nil, auth, nil, errInvalidAuthKey
	}
	
	session := deriveKeys(sha256.New, c.privkey, ephkey, auth.h.SrcID, c.localnode.ID(), cdata)
	session = session.keysFlipped()
	return n, auth, session, nil
}


func (c *Codec) decodeHandshakeAuthData(head *Header) (auth handshakeAuthData, err error) {
	
	if len(head.AuthData) < sizeofHandshakeAuthData {
		return auth, fmt.Errorf("header authsize %d too low for handshake", head.AuthSize)
	}
	c.reader.Reset(head.AuthData)
	binary.Read(&c.reader, binary.BigEndian, &auth.h)
	head.src = auth.h.SrcID

	
	var (
		vardata       = head.AuthData[sizeofHandshakeAuthData:]
		sigAndKeySize = int(auth.h.SigSize) + int(auth.h.PubkeySize)
		keyOffset     = int(auth.h.SigSize)
		recOffset     = keyOffset + int(auth.h.PubkeySize)
	)
	if len(vardata) < sigAndKeySize {
		return auth, errTooShort
	}
	auth.signature = vardata[:keyOffset]
	auth.pubkey = vardata[keyOffset:recOffset]
	auth.record = vardata[recOffset:]
	return auth, nil
}




func (c *Codec) decodeHandshakeRecord(local *enode.Node, wantID enode.ID, remote []byte) (*enode.Node, error) {
	node := local
	if len(remote) > 0 {
		var record enr.Record
		if err := rlp.DecodeBytes(remote, &record); err != nil {
			return nil, err
		}
		if local == nil || local.Seq() < record.Seq() {
			n, err := enode.New(enode.ValidSchemes, &record)
			if err != nil {
				return nil, fmt.Errorf("invalid node record: %v", err)
			}
			if n.ID() != wantID {
				return nil, fmt.Errorf("record in handshake has wrong ID: %v", n.ID())
			}
			node = n
		}
	}
	if node == nil {
		return nil, errNoRecord
	}
	return node, nil
}


func (c *Codec) decodeMessage(fromAddr string, head *Header, headerData, msgData []byte) (Packet, error) {
	if len(head.AuthData) != sizeofMessageAuthData {
		return nil, fmt.Errorf("invalid auth size %d for message packet", len(head.AuthData))
	}
	var auth messageAuthData
	c.reader.Reset(head.AuthData)
	binary.Read(&c.reader, binary.BigEndian, &auth)
	head.src = auth.SrcID

	
	key := c.sc.readKey(auth.SrcID, fromAddr)
	msg, err := c.decryptMessage(msgData, head.Nonce[:], headerData, key)
	if err == errMessageDecrypt {
		
		return &Unknown{Nonce: head.Nonce}, nil
	}
	return msg, err
}

func (c *Codec) decryptMessage(input, nonce, headerData, readKey []byte) (Packet, error) {
	msgdata, err := decryptGCM(readKey, nonce, input, headerData)
	if err != nil {
		return nil, errMessageDecrypt
	}
	if len(msgdata) == 0 {
		return nil, errMessageTooShort
	}
	return DecodeMessage(msgdata[0], msgdata[1:])
}



func (h *StaticHeader) checkValid(packetLen int) error {
	if h.ProtocolID != protocolID {
		return errInvalidHeader
	}
	if h.Version < minVersion {
		return errMinVersion
	}
	if h.Flag != flagWhoareyou && packetLen < minMessageSize {
		return errMsgTooShort
	}
	if int(h.AuthSize) > packetLen {
		return errAuthSize
	}
	return nil
}


func (h *Header) mask(destID enode.ID) cipher.Stream {
	block, err := aes.NewCipher(destID[:16])
	if err != nil {
		panic("can't create cipher")
	}
	return cipher.NewCTR(block, h.IV[:])
}

func bytesCopy(r *bytes.Buffer) []byte {
	b := make([]byte, r.Len())
	copy(b, r.Bytes())
	return b
}
