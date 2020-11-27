















package v4test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net"
	"reflect"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/internal/utesting"
	"github.com/ethereum/go-ethereum/p2p/discover/v4wire"
)

const (
	expiration  = 20 * time.Second
	wrongPacket = 66
	macSize     = 256 / 8
)

var (
	
	Remote string
	
	Listen1 string = "127.0.0.1"
	
	
	Listen2 string = "127.0.0.2"
)

type pingWithJunk struct {
	Version    uint
	From, To   v4wire.Endpoint
	Expiration uint64
	JunkData1  uint
	JunkData2  []byte
}

func (req *pingWithJunk) Name() string { return "PING/v4" }
func (req *pingWithJunk) Kind() byte   { return v4wire.PingPacket }

type pingWrongType struct {
	Version    uint
	From, To   v4wire.Endpoint
	Expiration uint64
}

func (req *pingWrongType) Name() string { return "WRONG/v4" }
func (req *pingWrongType) Kind() byte   { return wrongPacket }

func futureExpiration() uint64 {
	return uint64(time.Now().Add(expiration).Unix())
}


func BasicPing(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	pingHash := te.send(te.l1, &v4wire.Ping{
		Version:    4,
		From:       te.localEndpoint(te.l1),
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
	})

	reply, _, _ := te.read(te.l1)
	if err := te.checkPong(reply, pingHash); err != nil {
		t.Fatal(err)
	}
}


func (te *testenv) checkPong(reply v4wire.Packet, pingHash []byte) error {
	if reply == nil || reply.Kind() != v4wire.PongPacket {
		return fmt.Errorf("expected PONG reply, got %v", reply)
	}
	pong := reply.(*v4wire.Pong)
	if !bytes.Equal(pong.ReplyTok, pingHash) {
		return fmt.Errorf("PONG reply token mismatch: got %x, want %x", pong.ReplyTok, pingHash)
	}
	wantEndpoint := te.localEndpoint(te.l1)
	if !reflect.DeepEqual(pong.To, wantEndpoint) {
		return fmt.Errorf("PONG 'to' endpoint mismatch: got %+v, want %+v", pong.To, wantEndpoint)
	}
	if v4wire.Expired(pong.Expiration) {
		return fmt.Errorf("PONG is expired (%v)", pong.Expiration)
	}
	return nil
}


func PingWrongTo(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	wrongEndpoint := v4wire.Endpoint{IP: net.ParseIP("192.0.2.0")}
	pingHash := te.send(te.l1, &v4wire.Ping{
		Version:    4,
		From:       te.localEndpoint(te.l1),
		To:         wrongEndpoint,
		Expiration: futureExpiration(),
	})

	reply, _, _ := te.read(te.l1)
	if err := te.checkPong(reply, pingHash); err != nil {
		t.Fatal(err)
	}
}


func PingWrongFrom(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	wrongEndpoint := v4wire.Endpoint{IP: net.ParseIP("192.0.2.0")}
	pingHash := te.send(te.l1, &v4wire.Ping{
		Version:    4,
		From:       wrongEndpoint,
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
	})

	reply, _, _ := te.read(te.l1)
	if err := te.checkPong(reply, pingHash); err != nil {
		t.Fatal(err)
	}
}




func PingExtraData(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	pingHash := te.send(te.l1, &pingWithJunk{
		Version:    4,
		From:       te.localEndpoint(te.l1),
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
		JunkData1:  42,
		JunkData2:  []byte{9, 8, 7, 6, 5, 4, 3, 2, 1},
	})

	reply, _, _ := te.read(te.l1)
	if err := te.checkPong(reply, pingHash); err != nil {
		t.Fatal(err)
	}
}



func PingExtraDataWrongFrom(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	wrongEndpoint := v4wire.Endpoint{IP: net.ParseIP("192.0.2.0")}
	req := pingWithJunk{
		Version:    4,
		From:       wrongEndpoint,
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
		JunkData1:  42,
		JunkData2:  []byte{9, 8, 7, 6, 5, 4, 3, 2, 1},
	}
	pingHash := te.send(te.l1, &req)
	reply, _, _ := te.read(te.l1)
	if err := te.checkPong(reply, pingHash); err != nil {
		t.Fatal(err)
	}
}



func PingPastExpiration(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	te.send(te.l1, &v4wire.Ping{
		Version:    4,
		From:       te.localEndpoint(te.l1),
		To:         te.remoteEndpoint(),
		Expiration: -futureExpiration(),
	})

	reply, _, _ := te.read(te.l1)
	if reply != nil {
		t.Fatal("Expected no reply, got", reply)
	}
}


func WrongPacketType(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	te.send(te.l1, &pingWrongType{
		Version:    4,
		From:       te.localEndpoint(te.l1),
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
	})

	reply, _, _ := te.read(te.l1)
	if reply != nil {
		t.Fatal("Expected no reply, got", reply)
	}
}



func BondThenPingWithWrongFrom(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()
	bond(t, te)

	wrongEndpoint := v4wire.Endpoint{IP: net.ParseIP("192.0.2.0")}
	pingHash := te.send(te.l1, &v4wire.Ping{
		Version:    4,
		From:       wrongEndpoint,
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
	})

	reply, _, _ := te.read(te.l1)
	if err := te.checkPong(reply, pingHash); err != nil {
		t.Fatal(err)
	}
}



func FindnodeWithoutEndpointProof(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	req := v4wire.Findnode{Expiration: futureExpiration()}
	rand.Read(req.Target[:])
	te.send(te.l1, &req)

	reply, _, _ := te.read(te.l1)
	if reply != nil {
		t.Fatal("Expected no response, got", reply)
	}
}



func BasicFindnode(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()
	bond(t, te)

	findnode := v4wire.Findnode{Expiration: futureExpiration()}
	rand.Read(findnode.Target[:])
	te.send(te.l1, &findnode)

	reply, _, err := te.read(te.l1)
	if err != nil {
		t.Fatal("read find nodes", err)
	}
	if reply.Kind() != v4wire.NeighborsPacket {
		t.Fatal("Expected neighbors, got", reply.Name())
	}
}




func UnsolicitedNeighbors(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()
	bond(t, te)

	
	fakeKey, _ := crypto.GenerateKey()
	encFakeKey := v4wire.EncodePubkey(&fakeKey.PublicKey)
	neighbors := v4wire.Neighbors{
		Expiration: futureExpiration(),
		Nodes: []v4wire.Node{{
			ID:  encFakeKey,
			IP:  net.IP{1, 2, 3, 4},
			UDP: 30303,
			TCP: 30303,
		}},
	}
	te.send(te.l1, &neighbors)

	
	te.send(te.l1, &v4wire.Findnode{
		Expiration: futureExpiration(),
		Target:     encFakeKey,
	})

	reply, _, err := te.read(te.l1)
	if err != nil {
		t.Fatal("read find nodes", err)
	}
	if reply.Kind() != v4wire.NeighborsPacket {
		t.Fatal("Expected neighbors, got", reply.Name())
	}
	nodes := reply.(*v4wire.Neighbors).Nodes
	if contains(nodes, encFakeKey) {
		t.Fatal("neighbors response contains node from earlier unsolicited neighbors response")
	}
}



func FindnodePastExpiration(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()
	bond(t, te)

	findnode := v4wire.Findnode{Expiration: -futureExpiration()}
	rand.Read(findnode.Target[:])
	te.send(te.l1, &findnode)

	for {
		reply, _, _ := te.read(te.l1)
		if reply == nil {
			return
		} else if reply.Kind() == v4wire.NeighborsPacket {
			t.Fatal("Unexpected NEIGHBORS response for expired FINDNODE request")
		}
	}
}


func bond(t *utesting.T, te *testenv) {
	te.send(te.l1, &v4wire.Ping{
		Version:    4,
		From:       te.localEndpoint(te.l1),
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
	})

	var gotPing, gotPong bool
	for !gotPing || !gotPong {
		req, hash, err := te.read(te.l1)
		if err != nil {
			t.Fatal(err)
		}
		switch req.(type) {
		case *v4wire.Ping:
			te.send(te.l1, &v4wire.Pong{
				To:         te.remoteEndpoint(),
				ReplyTok:   hash,
				Expiration: futureExpiration(),
			})
			gotPing = true
		case *v4wire.Pong:
			
			gotPong = true
		}
	}
}








func FindnodeAmplificationInvalidPongHash(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	
	te.send(te.l1, &v4wire.Ping{
		Version:    4,
		From:       te.localEndpoint(te.l1),
		To:         te.remoteEndpoint(),
		Expiration: futureExpiration(),
	})

	var gotPing, gotPong bool
	for !gotPing || !gotPong {
		req, _, err := te.read(te.l1)
		if err != nil {
			t.Fatal(err)
		}
		switch req.(type) {
		case *v4wire.Ping:
			
			te.send(te.l1, &v4wire.Pong{
				To:         te.remoteEndpoint(),
				ReplyTok:   make([]byte, macSize),
				Expiration: futureExpiration(),
			})
			gotPing = true
		case *v4wire.Pong:
			gotPong = true
		}
	}

	
	
	findnode := v4wire.Findnode{Expiration: futureExpiration()}
	rand.Read(findnode.Target[:])
	te.send(te.l1, &findnode)

	
	reply, _, _ := te.read(te.l1)
	if reply != nil && reply.Kind() == v4wire.NeighborsPacket {
		t.Error("Got neighbors")
	}
}




func FindnodeAmplificationWrongIP(t *utesting.T) {
	te := newTestEnv(Remote, Listen1, Listen2)
	defer te.close()

	
	bond(t, te)

	
	
	findnode := v4wire.Findnode{Expiration: futureExpiration()}
	rand.Read(findnode.Target[:])
	te.send(te.l2, &findnode)

	
	reply, _, _ := te.read(te.l2)
	if reply != nil {
		t.Error("Got NEIGHORS response for FINDNODE from wrong IP")
	}
}

var AllTests = []utesting.Test{
	{Name: "Ping/Basic", Fn: BasicPing},
	{Name: "Ping/WrongTo", Fn: PingWrongTo},
	{Name: "Ping/WrongFrom", Fn: PingWrongFrom},
	{Name: "Ping/ExtraData", Fn: PingExtraData},
	{Name: "Ping/ExtraDataWrongFrom", Fn: PingExtraDataWrongFrom},
	{Name: "Ping/PastExpiration", Fn: PingPastExpiration},
	{Name: "Ping/WrongPacketType", Fn: WrongPacketType},
	{Name: "Ping/BondThenPingWithWrongFrom", Fn: BondThenPingWithWrongFrom},
	{Name: "Findnode/WithoutEndpointProof", Fn: FindnodeWithoutEndpointProof},
	{Name: "Findnode/BasicFindnode", Fn: BasicFindnode},
	{Name: "Findnode/UnsolicitedNeighbors", Fn: UnsolicitedNeighbors},
	{Name: "Findnode/PastExpiration", Fn: FindnodePastExpiration},
	{Name: "Amplification/InvalidPongHash", Fn: FindnodeAmplificationInvalidPongHash},
	{Name: "Amplification/WrongIP", Fn: FindnodeAmplificationWrongIP},
}
