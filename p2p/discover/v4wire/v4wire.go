
















package v4wire

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
)


const (
	PingPacket = iota + 1 
	PongPacket
	FindnodePacket
	NeighborsPacket
	ENRRequestPacket
	ENRResponsePacket
)


type (
	Ping struct {
		Version    uint
		From, To   Endpoint
		Expiration uint64
		
		Rest []rlp.RawValue `rlp:"tail"`
	}

	
	Pong struct {
		
		
		
		To         Endpoint
		ReplyTok   []byte 
		Expiration uint64 
		
		Rest []rlp.RawValue `rlp:"tail"`
	}

	
	Findnode struct {
		Target     Pubkey
		Expiration uint64
		
		Rest []rlp.RawValue `rlp:"tail"`
	}

	
	Neighbors struct {
		Nodes      []Node
		Expiration uint64
		
		Rest []rlp.RawValue `rlp:"tail"`
	}

	
	ENRRequest struct {
		Expiration uint64
		
		Rest []rlp.RawValue `rlp:"tail"`
	}

	
	ENRResponse struct {
		ReplyTok []byte 
		Record   enr.Record
		
		Rest []rlp.RawValue `rlp:"tail"`
	}
)


const MaxNeighbors = 12























type Pubkey [64]byte


func (e Pubkey) ID() enode.ID {
	return enode.ID(crypto.Keccak256Hash(e[:]))
}


type Node struct {
	IP  net.IP 
	UDP uint16 
	TCP uint16 
	ID  Pubkey
}


type Endpoint struct {
	IP  net.IP 
	UDP uint16 
	TCP uint16 
}


func NewEndpoint(addr *net.UDPAddr, tcpPort uint16) Endpoint {
	ip := net.IP{}
	if ip4 := addr.IP.To4(); ip4 != nil {
		ip = ip4
	} else if ip6 := addr.IP.To16(); ip6 != nil {
		ip = ip6
	}
	return Endpoint{IP: ip, UDP: uint16(addr.Port), TCP: tcpPort}
}

type Packet interface {
	
	Name() string
	Kind() byte
}

func (req *Ping) Name() string   { return "PING/v4" }
func (req *Ping) Kind() byte     { return PingPacket }
func (req *Ping) ENRSeq() uint64 { return seqFromTail(req.Rest) }

func (req *Pong) Name() string   { return "PONG/v4" }
func (req *Pong) Kind() byte     { return PongPacket }
func (req *Pong) ENRSeq() uint64 { return seqFromTail(req.Rest) }

func (req *Findnode) Name() string { return "FINDNODE/v4" }
func (req *Findnode) Kind() byte   { return FindnodePacket }

func (req *Neighbors) Name() string { return "NEIGHBORS/v4" }
func (req *Neighbors) Kind() byte   { return NeighborsPacket }

func (req *ENRRequest) Name() string { return "ENRREQUEST/v4" }
func (req *ENRRequest) Kind() byte   { return ENRRequestPacket }

func (req *ENRResponse) Name() string { return "ENRRESPONSE/v4" }
func (req *ENRResponse) Kind() byte   { return ENRResponsePacket }


func Expired(ts uint64) bool {
	return time.Unix(int64(ts), 0).Before(time.Now())
}

func seqFromTail(tail []rlp.RawValue) uint64 {
	if len(tail) == 0 {
		return 0
	}
	var seq uint64
	rlp.DecodeBytes(tail[0], &seq)
	return seq
}



const (
	macSize  = 32
	sigSize  = crypto.SignatureLength
	headSize = macSize + sigSize 
)

var (
	ErrPacketTooSmall = errors.New("too small")
	ErrBadHash        = errors.New("bad hash")
	ErrBadPoint       = errors.New("invalid curve point")
)

var headSpace = make([]byte, headSize)


func Decode(input []byte) (Packet, Pubkey, []byte, error) {
	if len(input) < headSize+1 {
		return nil, Pubkey{}, nil, ErrPacketTooSmall
	}
	hash, sig, sigdata := input[:macSize], input[macSize:headSize], input[headSize:]
	shouldhash := crypto.Keccak256(input[macSize:])
	if !bytes.Equal(hash, shouldhash) {
		return nil, Pubkey{}, nil, ErrBadHash
	}
	fromKey, err := recoverNodeKey(crypto.Keccak256(input[headSize:]), sig)
	if err != nil {
		return nil, fromKey, hash, err
	}

	var req Packet
	switch ptype := sigdata[0]; ptype {
	case PingPacket:
		req = new(Ping)
	case PongPacket:
		req = new(Pong)
	case FindnodePacket:
		req = new(Findnode)
	case NeighborsPacket:
		req = new(Neighbors)
	case ENRRequestPacket:
		req = new(ENRRequest)
	case ENRResponsePacket:
		req = new(ENRResponse)
	default:
		return nil, fromKey, hash, fmt.Errorf("unknown type: %d", ptype)
	}
	s := rlp.NewStream(bytes.NewReader(sigdata[1:]), 0)
	err = s.Decode(req)
	return req, fromKey, hash, err
}


func Encode(priv *ecdsa.PrivateKey, req Packet) (packet, hash []byte, err error) {
	b := new(bytes.Buffer)
	b.Write(headSpace)
	b.WriteByte(req.Kind())
	if err := rlp.Encode(b, req); err != nil {
		return nil, nil, err
	}
	packet = b.Bytes()
	sig, err := crypto.Sign(crypto.Keccak256(packet[headSize:]), priv)
	if err != nil {
		return nil, nil, err
	}
	copy(packet[macSize:], sig)
	
	hash = crypto.Keccak256(packet[macSize:])
	copy(packet, hash)
	return packet, hash, nil
}


func recoverNodeKey(hash, sig []byte) (key Pubkey, err error) {
	pubkey, err := crypto.Ecrecover(hash, sig)
	if err != nil {
		return key, err
	}
	copy(key[:], pubkey[1:])
	return key, nil
}


func EncodePubkey(key *ecdsa.PublicKey) Pubkey {
	var e Pubkey
	math.ReadBits(key.X, e[:len(e)/2])
	math.ReadBits(key.Y, e[len(e)/2:])
	return e
}


func DecodePubkey(curve elliptic.Curve, e Pubkey) (*ecdsa.PublicKey, error) {
	p := &ecdsa.PublicKey{Curve: curve, X: new(big.Int), Y: new(big.Int)}
	half := len(e) / 2
	p.X.SetBytes(e[:half])
	p.Y.SetBytes(e[half:])
	if !p.Curve.IsOnCurve(p.X, p.Y) {
		return nil, ErrBadPoint
	}
	return p, nil
}
