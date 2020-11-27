















package v5wire

import (
	"fmt"
	"net"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
)


type Packet interface {
	Name() string        
	Kind() byte          
	RequestID() []byte   
	SetRequestID([]byte) 
}


const (
	PingMsg byte = iota + 1
	PongMsg
	FindnodeMsg
	NodesMsg
	TalkRequestMsg
	TalkResponseMsg
	RequestTicketMsg
	TicketMsg
	RegtopicMsg
	RegconfirmationMsg
	TopicQueryMsg

	UnknownPacket   = byte(255) 
	WhoareyouPacket = byte(254) 
)


type (
	
	Unknown struct {
		Nonce Nonce
	}

	
	Whoareyou struct {
		ChallengeData []byte   
		Nonce         Nonce    
		IDNonce       [16]byte 
		RecordSeq     uint64   

		
		
		Node *enode.Node

		sent mclock.AbsTime 
	}

	
	Ping struct {
		ReqID  []byte
		ENRSeq uint64
	}

	
	Pong struct {
		ReqID  []byte
		ENRSeq uint64
		ToIP   net.IP 
		ToPort uint16 
	}

	
	Findnode struct {
		ReqID     []byte
		Distances []uint
	}

	
	Nodes struct {
		ReqID []byte
		Total uint8
		Nodes []*enr.Record
	}

	
	TalkRequest struct {
		ReqID    []byte
		Protocol string
		Message  []byte
	}

	
	TalkResponse struct {
		ReqID   []byte
		Message []byte
	}

	
	RequestTicket struct {
		ReqID []byte
		Topic []byte
	}

	
	Ticket struct {
		ReqID  []byte
		Ticket []byte
	}

	
	Regtopic struct {
		ReqID  []byte
		Ticket []byte
		ENR    *enr.Record
	}

	
	Regconfirmation struct {
		ReqID      []byte
		Registered bool
	}

	
	TopicQuery struct {
		ReqID []byte
		Topic []byte
	}
)


func DecodeMessage(ptype byte, body []byte) (Packet, error) {
	var dec Packet
	switch ptype {
	case PingMsg:
		dec = new(Ping)
	case PongMsg:
		dec = new(Pong)
	case FindnodeMsg:
		dec = new(Findnode)
	case NodesMsg:
		dec = new(Nodes)
	case TalkRequestMsg:
		dec = new(TalkRequest)
	case TalkResponseMsg:
		dec = new(TalkResponse)
	case RequestTicketMsg:
		dec = new(RequestTicket)
	case TicketMsg:
		dec = new(Ticket)
	case RegtopicMsg:
		dec = new(Regtopic)
	case RegconfirmationMsg:
		dec = new(Regconfirmation)
	case TopicQueryMsg:
		dec = new(TopicQuery)
	default:
		return nil, fmt.Errorf("unknown packet type %d", ptype)
	}
	if err := rlp.DecodeBytes(body, dec); err != nil {
		return nil, err
	}
	if dec.RequestID() != nil && len(dec.RequestID()) > 8 {
		return nil, ErrInvalidReqID
	}
	return dec, nil
}

func (*Whoareyou) Name() string        { return "WHOAREYOU/v5" }
func (*Whoareyou) Kind() byte          { return WhoareyouPacket }
func (*Whoareyou) RequestID() []byte   { return nil }
func (*Whoareyou) SetRequestID([]byte) {}

func (*Unknown) Name() string        { return "UNKNOWN/v5" }
func (*Unknown) Kind() byte          { return UnknownPacket }
func (*Unknown) RequestID() []byte   { return nil }
func (*Unknown) SetRequestID([]byte) {}

func (*Ping) Name() string             { return "PING/v5" }
func (*Ping) Kind() byte               { return PingMsg }
func (p *Ping) RequestID() []byte      { return p.ReqID }
func (p *Ping) SetRequestID(id []byte) { p.ReqID = id }

func (*Pong) Name() string             { return "PONG/v5" }
func (*Pong) Kind() byte               { return PongMsg }
func (p *Pong) RequestID() []byte      { return p.ReqID }
func (p *Pong) SetRequestID(id []byte) { p.ReqID = id }

func (*Findnode) Name() string             { return "FINDNODE/v5" }
func (*Findnode) Kind() byte               { return FindnodeMsg }
func (p *Findnode) RequestID() []byte      { return p.ReqID }
func (p *Findnode) SetRequestID(id []byte) { p.ReqID = id }

func (*Nodes) Name() string             { return "NODES/v5" }
func (*Nodes) Kind() byte               { return NodesMsg }
func (p *Nodes) RequestID() []byte      { return p.ReqID }
func (p *Nodes) SetRequestID(id []byte) { p.ReqID = id }

func (*TalkRequest) Name() string             { return "TALKREQ/v5" }
func (*TalkRequest) Kind() byte               { return TalkRequestMsg }
func (p *TalkRequest) RequestID() []byte      { return p.ReqID }
func (p *TalkRequest) SetRequestID(id []byte) { p.ReqID = id }

func (*TalkResponse) Name() string             { return "TALKRESP/v5" }
func (*TalkResponse) Kind() byte               { return TalkResponseMsg }
func (p *TalkResponse) RequestID() []byte      { return p.ReqID }
func (p *TalkResponse) SetRequestID(id []byte) { p.ReqID = id }

func (*RequestTicket) Name() string             { return "REQTICKET/v5" }
func (*RequestTicket) Kind() byte               { return RequestTicketMsg }
func (p *RequestTicket) RequestID() []byte      { return p.ReqID }
func (p *RequestTicket) SetRequestID(id []byte) { p.ReqID = id }

func (*Regtopic) Name() string             { return "REGTOPIC/v5" }
func (*Regtopic) Kind() byte               { return RegtopicMsg }
func (p *Regtopic) RequestID() []byte      { return p.ReqID }
func (p *Regtopic) SetRequestID(id []byte) { p.ReqID = id }

func (*Ticket) Name() string             { return "TICKET/v5" }
func (*Ticket) Kind() byte               { return TicketMsg }
func (p *Ticket) RequestID() []byte      { return p.ReqID }
func (p *Ticket) SetRequestID(id []byte) { p.ReqID = id }

func (*Regconfirmation) Name() string             { return "REGCONFIRMATION/v5" }
func (*Regconfirmation) Kind() byte               { return RegconfirmationMsg }
func (p *Regconfirmation) RequestID() []byte      { return p.ReqID }
func (p *Regconfirmation) SetRequestID(id []byte) { p.ReqID = id }

func (*TopicQuery) Name() string             { return "TOPICQUERY/v5" }
func (*TopicQuery) Kind() byte               { return TopicQueryMsg }
func (p *TopicQuery) RequestID() []byte      { return p.ReqID }
func (p *TopicQuery) SetRequestID(id []byte) { p.ReqID = id }
