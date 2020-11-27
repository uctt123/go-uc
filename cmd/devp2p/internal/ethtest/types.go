















package ethtest

import (
	"crypto/ecdsa"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/utesting"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/rlpx"
	"github.com/ethereum/go-ethereum/rlp"
)

type Message interface {
	Code() int
}

type Error struct {
	err error
}

func (e *Error) Unwrap() error    { return e.err }
func (e *Error) Error() string    { return e.err.Error() }
func (e *Error) Code() int        { return -1 }
func (e *Error) GoString() string { return e.Error() }


type Hello struct {
	Version    uint64
	Name       string
	Caps       []p2p.Cap
	ListenPort uint64
	ID         []byte 

	
	Rest []rlp.RawValue `rlp:"tail"`
}

func (h Hello) Code() int { return 0x00 }


type Disconnect struct {
	Reason p2p.DiscReason
}

func (d Disconnect) Code() int { return 0x01 }

type Ping struct{}

func (p Ping) Code() int { return 0x02 }

type Pong struct{}

func (p Pong) Code() int { return 0x03 }


type Status struct {
	ProtocolVersion uint32
	NetworkID       uint64
	TD              *big.Int
	Head            common.Hash
	Genesis         common.Hash
	ForkID          forkid.ID
}

func (s Status) Code() int { return 16 }


type NewBlockHashes []struct {
	Hash   common.Hash 
	Number uint64      
}

func (nbh NewBlockHashes) Code() int { return 17 }


type NewBlock struct {
	Block *types.Block
	TD    *big.Int
}

func (nb NewBlock) Code() int { return 23 }


type GetBlockHeaders struct {
	Origin  hashOrNumber 
	Amount  uint64       
	Skip    uint64       
	Reverse bool         
}

func (g GetBlockHeaders) Code() int { return 19 }

type BlockHeaders []*types.Header

func (bh BlockHeaders) Code() int { return 20 }


type hashOrNumber struct {
	Hash   common.Hash 
	Number uint64      
}



func (hn *hashOrNumber) EncodeRLP(w io.Writer) error {
	if hn.Hash == (common.Hash{}) {
		return rlp.Encode(w, hn.Number)
	}
	if hn.Number != 0 {
		return fmt.Errorf("both origin hash (%x) and number (%d) provided", hn.Hash, hn.Number)
	}
	return rlp.Encode(w, hn.Hash)
}



func (hn *hashOrNumber) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	origin, err := s.Raw()
	if err == nil {
		switch {
		case size == 32:
			err = rlp.DecodeBytes(origin, &hn.Hash)
		case size <= 8:
			err = rlp.DecodeBytes(origin, &hn.Number)
		default:
			err = fmt.Errorf("invalid input size %d for origin", size)
		}
	}
	return err
}


type GetBlockBodies []common.Hash

func (gbb GetBlockBodies) Code() int { return 21 }


type BlockBodies []*types.Body

func (bb BlockBodies) Code() int { return 22 }


type Conn struct {
	*rlpx.Conn
	ourKey             *ecdsa.PrivateKey
	ethProtocolVersion uint
}

func (c *Conn) Read() Message {
	code, rawData, _, err := c.Conn.Read()
	if err != nil {
		return &Error{fmt.Errorf("could not read from connection: %v", err)}
	}

	var msg Message
	switch int(code) {
	case (Hello{}).Code():
		msg = new(Hello)
	case (Ping{}).Code():
		msg = new(Ping)
	case (Pong{}).Code():
		msg = new(Pong)
	case (Disconnect{}).Code():
		msg = new(Disconnect)
	case (Status{}).Code():
		msg = new(Status)
	case (GetBlockHeaders{}).Code():
		msg = new(GetBlockHeaders)
	case (BlockHeaders{}).Code():
		msg = new(BlockHeaders)
	case (GetBlockBodies{}).Code():
		msg = new(GetBlockBodies)
	case (BlockBodies{}).Code():
		msg = new(BlockBodies)
	case (NewBlock{}).Code():
		msg = new(NewBlock)
	case (NewBlockHashes{}).Code():
		msg = new(NewBlockHashes)
	default:
		return &Error{fmt.Errorf("invalid message code: %d", code)}
	}

	if err := rlp.DecodeBytes(rawData, msg); err != nil {
		return &Error{fmt.Errorf("could not rlp decode message: %v", err)}
	}

	return msg
}



func (c *Conn) ReadAndServe(chain *Chain) Message {
	for {
		switch msg := c.Read().(type) {
		case *Ping:
			c.Write(&Pong{})
		case *GetBlockHeaders:
			req := *msg
			headers, err := chain.GetHeaders(req)
			if err != nil {
				return &Error{fmt.Errorf("could not get headers for inbound header request: %v", err)}
			}

			if err := c.Write(headers); err != nil {
				return &Error{fmt.Errorf("could not write to connection: %v", err)}
			}
		default:
			return msg
		}
	}
}

func (c *Conn) Write(msg Message) error {
	payload, err := rlp.EncodeToBytes(msg)
	if err != nil {
		return err
	}
	_, err = c.Conn.Write(uint64(msg.Code()), payload)
	return err

}


func (c *Conn) handshake(t *utesting.T) Message {
	
	pub0 := crypto.FromECDSAPub(&c.ourKey.PublicKey)[1:]
	ourHandshake := &Hello{
		Version: 5,
		Caps: []p2p.Cap{
			{Name: "eth", Version: 64},
			{Name: "eth", Version: 65},
		},
		ID: pub0,
	}
	if err := c.Write(ourHandshake); err != nil {
		t.Fatalf("could not write to connection: %v", err)
	}
	
	switch msg := c.Read().(type) {
	case *Hello:
		
		if msg.Version >= 5 {
			c.SetSnappy(true)
		}

		c.negotiateEthProtocol(msg.Caps)
		if c.ethProtocolVersion == 0 {
			t.Fatalf("unexpected eth protocol version")
		}
		return msg
	default:
		t.Fatalf("bad handshake: %#v", msg)
		return nil
	}
}



func (c *Conn) negotiateEthProtocol(caps []p2p.Cap) {
	var highestEthVersion uint
	for _, capability := range caps {
		if capability.Name != "eth" {
			continue
		}
		if capability.Version > highestEthVersion && capability.Version <= 65 {
			highestEthVersion = capability.Version
		}
	}
	c.ethProtocolVersion = highestEthVersion
}



func (c *Conn) statusExchange(t *utesting.T, chain *Chain) Message {
	
	var message Message

loop:
	for {
		switch msg := c.Read().(type) {
		case *Status:
			if msg.Head != chain.blocks[chain.Len()-1].Hash() {
				t.Fatalf("wrong head in status: %v", msg.Head)
			}
			if msg.TD.Cmp(chain.TD(chain.Len())) != 0 {
				t.Fatalf("wrong TD in status: %v", msg.TD)
			}
			if !reflect.DeepEqual(msg.ForkID, chain.ForkID()) {
				t.Fatalf("wrong fork ID in status: %v", msg.ForkID)
			}
			message = msg
			break loop
		case *Disconnect:
			t.Fatalf("disconnect received: %v", msg.Reason)
		case *Ping:
			c.Write(&Pong{}) 
			
		default:
			t.Fatalf("bad status message: %#v", msg)
		}
	}
	
	if c.ethProtocolVersion == 0 {
		t.Fatalf("eth protocol version must be set in Conn")
	}
	
	status := Status{
		ProtocolVersion: uint32(c.ethProtocolVersion),
		NetworkID:       1,
		TD:              chain.TD(chain.Len()),
		Head:            chain.blocks[chain.Len()-1].Hash(),
		Genesis:         chain.blocks[0].Hash(),
		ForkID:          chain.ForkID(),
	}
	if err := c.Write(status); err != nil {
		t.Fatalf("could not write to connection: %v", err)
	}

	return message
}



func (c *Conn) waitForBlock(block *types.Block) error {
	for {
		req := &GetBlockHeaders{Origin: hashOrNumber{Hash: block.Hash()}, Amount: 1}
		if err := c.Write(req); err != nil {
			return err
		}

		switch msg := c.Read().(type) {
		case *BlockHeaders:
			if len(*msg) > 0 {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		default:
			return fmt.Errorf("invalid message: %v", msg)
		}
	}
}
