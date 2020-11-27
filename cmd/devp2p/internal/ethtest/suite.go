















package ethtest

import (
	"fmt"
	"net"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/utesting"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/rlpx"
	"github.com/stretchr/testify/assert"
)



type Suite struct {
	Dest *enode.Node

	chain     *Chain
	fullChain *Chain
}




func NewSuite(dest *enode.Node, chainfile string, genesisfile string) *Suite {
	chain, err := loadChain(chainfile, genesisfile)
	if err != nil {
		panic(err)
	}
	return &Suite{
		Dest:      dest,
		chain:     chain.Shorten(1000),
		fullChain: chain,
	}
}

func (s *Suite) AllTests() []utesting.Test {
	return []utesting.Test{
		{Name: "Status", Fn: s.TestStatus},
		{Name: "GetBlockHeaders", Fn: s.TestGetBlockHeaders},
		{Name: "Broadcast", Fn: s.TestBroadcast},
		{Name: "GetBlockBodies", Fn: s.TestGetBlockBodies},
	}
}




func (s *Suite) TestStatus(t *utesting.T) {
	conn, err := s.dial()
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}
	
	conn.handshake(t)
	
	switch msg := conn.statusExchange(t, s.chain).(type) {
	case *Status:
		t.Logf("%+v\n", msg)
	default:
		t.Fatalf("unexpected: %#v", msg)
	}
}



func (s *Suite) TestGetBlockHeaders(t *utesting.T) {
	conn, err := s.dial()
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}

	conn.handshake(t)
	conn.statusExchange(t, s.chain)

	
	req := &GetBlockHeaders{
		Origin: hashOrNumber{
			Hash: s.chain.blocks[1].Hash(),
		},
		Amount:  2,
		Skip:    1,
		Reverse: false,
	}

	if err := conn.Write(req); err != nil {
		t.Fatalf("could not write to connection: %v", err)
	}

	switch msg := conn.ReadAndServe(s.chain).(type) {
	case *BlockHeaders:
		headers := msg
		for _, header := range *headers {
			num := header.Number.Uint64()
			assert.Equal(t, s.chain.blocks[int(num)].Header(), header)
			t.Logf("\nHEADER FOR BLOCK NUMBER %d: %+v\n", header.Number, header)
		}
	default:
		t.Fatalf("unexpected: %#v", msg)
	}
}



func (s *Suite) TestGetBlockBodies(t *utesting.T) {
	conn, err := s.dial()
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}

	conn.handshake(t)
	conn.statusExchange(t, s.chain)
	
	req := &GetBlockBodies{s.chain.blocks[54].Hash(), s.chain.blocks[75].Hash()}
	if err := conn.Write(req); err != nil {
		t.Fatalf("could not write to connection: %v", err)
	}

	switch msg := conn.ReadAndServe(s.chain).(type) {
	case *BlockBodies:
		bodies := msg
		for _, body := range *bodies {
			t.Logf("\nBODY: %+v\n", body)
		}
	default:
		t.Fatalf("unexpected: %#v", msg)
	}
}



func (s *Suite) TestBroadcast(t *utesting.T) {
	
	sendConn, err := s.dial()
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}
	
	receiveConn, err := s.dial()
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}

	sendConn.handshake(t)
	receiveConn.handshake(t)

	sendConn.statusExchange(t, s.chain)
	receiveConn.statusExchange(t, s.chain)

	
	blockAnnouncement := &NewBlock{
		Block: s.fullChain.blocks[1000],
		TD:    s.fullChain.TD(1001),
	}
	if err := sendConn.Write(blockAnnouncement); err != nil {
		t.Fatalf("could not write to connection: %v", err)
	}

	switch msg := receiveConn.ReadAndServe(s.chain).(type) {
	case *NewBlock:
		assert.Equal(t, blockAnnouncement.Block.Header(), msg.Block.Header(),
			"wrong block header in announcement")
		assert.Equal(t, blockAnnouncement.TD, msg.TD,
			"wrong TD in announcement")
	case *NewBlockHashes:
		hashes := *msg
		assert.Equal(t, blockAnnouncement.Block.Hash(), hashes[0].Hash,
			"wrong block hash in announcement")
	default:
		t.Fatalf("unexpected: %#v", msg)
	}
	
	s.chain.blocks = append(s.chain.blocks, s.fullChain.blocks[1000])
	
	if err := receiveConn.waitForBlock(s.chain.Head()); err != nil {
		t.Fatal(err)
	}
}



func (s *Suite) dial() (*Conn, error) {
	var conn Conn

	fd, err := net.Dial("tcp", fmt.Sprintf("%v:%d", s.Dest.IP(), s.Dest.TCP()))
	if err != nil {
		return nil, err
	}
	conn.Conn = rlpx.NewConn(fd, s.Dest.Pubkey())

	
	conn.ourKey, _ = crypto.GenerateKey()
	_, err = conn.Handshake(conn.ourKey)
	if err != nil {
		return nil, err
	}

	return &conn, nil
}
