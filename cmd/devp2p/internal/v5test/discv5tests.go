















package v5test

import (
	"bytes"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/internal/utesting"
	"github.com/ethereum/go-ethereum/p2p/discover/v5wire"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)


type Suite struct {
	Dest             *enode.Node
	Listen1, Listen2 string 
}

func (s *Suite) listen1(log logger) (*conn, net.PacketConn) {
	c := newConn(s.Dest, log)
	l := c.listen(s.Listen1)
	return c, l
}

func (s *Suite) listen2(log logger) (*conn, net.PacketConn, net.PacketConn) {
	c := newConn(s.Dest, log)
	l1, l2 := c.listen(s.Listen1), c.listen(s.Listen2)
	return c, l1, l2
}

func (s *Suite) AllTests() []utesting.Test {
	return []utesting.Test{
		{Name: "Ping", Fn: s.TestPing},
		{Name: "PingLargeRequestID", Fn: s.TestPingLargeRequestID},
		{Name: "PingMultiIP", Fn: s.TestPingMultiIP},
		{Name: "PingHandshakeInterrupted", Fn: s.TestPingHandshakeInterrupted},
		{Name: "TalkRequest", Fn: s.TestTalkRequest},
		{Name: "FindnodeZeroDistance", Fn: s.TestFindnodeZeroDistance},
		{Name: "FindnodeResults", Fn: s.TestFindnodeResults},
	}
}


func (s *Suite) TestPing(t *utesting.T) {
	conn, l1 := s.listen1(t)
	defer conn.close()

	ping := &v5wire.Ping{ReqID: conn.nextReqID()}
	switch resp := conn.reqresp(l1, ping).(type) {
	case *v5wire.Pong:
		checkPong(t, resp, ping, l1)
	default:
		t.Fatal("expected PONG, got", resp.Name())
	}
}

func checkPong(t *utesting.T, pong *v5wire.Pong, ping *v5wire.Ping, c net.PacketConn) {
	if !bytes.Equal(pong.ReqID, ping.ReqID) {
		t.Fatalf("wrong request ID %x in PONG, want %x", pong.ReqID, ping.ReqID)
	}
	if !pong.ToIP.Equal(laddr(c).IP) {
		t.Fatalf("wrong destination IP %v in PONG, want %v", pong.ToIP, laddr(c).IP)
	}
	if int(pong.ToPort) != laddr(c).Port {
		t.Fatalf("wrong destination port %v in PONG, want %v", pong.ToPort, laddr(c).Port)
	}
}



func (s *Suite) TestPingLargeRequestID(t *utesting.T) {
	conn, l1 := s.listen1(t)
	defer conn.close()

	ping := &v5wire.Ping{ReqID: make([]byte, 9)}
	switch resp := conn.reqresp(l1, ping).(type) {
	case *v5wire.Pong:
		t.Errorf("PONG response with unknown request ID %x", resp.ReqID)
	case *readError:
		if resp.err == v5wire.ErrInvalidReqID {
			t.Error("response with oversized request ID")
		} else if !netutil.IsTimeout(resp.err) {
			t.Error(resp)
		}
	}
}




func (s *Suite) TestPingMultiIP(t *utesting.T) {
	conn, l1, l2 := s.listen2(t)
	defer conn.close()

	
	ping := &v5wire.Ping{ReqID: conn.nextReqID()}
	resp := conn.reqresp(l1, ping)
	if resp.Kind() != v5wire.PongMsg {
		t.Fatal("expected PONG, got", resp)
	}
	checkPong(t, resp.(*v5wire.Pong), ping, l1)

	
	ping2 := &v5wire.Ping{ReqID: conn.nextReqID()}
	conn.write(l2, ping2, nil)
	switch resp := conn.read(l2).(type) {
	case *v5wire.Pong:
		t.Fatalf("remote responded to PING from %v for session on IP %v", laddr(l2).IP, laddr(l1).IP)
	case *v5wire.Whoareyou:
		t.Logf("got WHOAREYOU for new session as expected")
		resp.Node = s.Dest
		conn.write(l2, ping2, resp)
	default:
		t.Fatal("expected WHOAREYOU, got", resp)
	}

	
	switch resp := conn.read(l2).(type) {
	case *v5wire.Pong:
		checkPong(t, resp, ping2, l2)
	default:
		t.Fatal("expected PONG, got", resp)
	}

	
	ping3 := &v5wire.Ping{ReqID: conn.nextReqID()}
	conn.write(l1, ping3, nil)
	switch resp := conn.read(l1).(type) {
	case *v5wire.Pong:
		t.Fatalf("remote responded to PING from %v for session on IP %v", laddr(l1).IP, laddr(l2).IP)
	case *v5wire.Whoareyou:
		t.Logf("got WHOAREYOU for new session as expected")
	default:
		t.Fatal("expected WHOAREYOU, got", resp)
	}
}




func (s *Suite) TestPingHandshakeInterrupted(t *utesting.T) {
	conn, l1 := s.listen1(t)
	defer conn.close()

	
	ping := &v5wire.Ping{ReqID: conn.nextReqID()}
	conn.write(l1, ping, nil)
	switch resp := conn.read(l1).(type) {
	case *v5wire.Whoareyou:
		t.Logf("got WHOAREYOU for PING")
	default:
		t.Fatal("expected WHOAREYOU, got", resp)
	}

	
	ping2 := &v5wire.Ping{ReqID: conn.nextReqID()}
	switch resp := conn.reqresp(l1, ping2).(type) {
	case *v5wire.Pong:
		checkPong(t, resp, ping2, l1)
	default:
		t.Fatal("expected WHOAREYOU, got", resp)
	}
}


func (s *Suite) TestTalkRequest(t *utesting.T) {
	conn, l1 := s.listen1(t)
	defer conn.close()

	
	id := conn.nextReqID()
	resp := conn.reqresp(l1, &v5wire.TalkRequest{ReqID: id, Protocol: "test-protocol"})
	switch resp := resp.(type) {
	case *v5wire.TalkResponse:
		if !bytes.Equal(resp.ReqID, id) {
			t.Fatalf("wrong request ID %x in TALKRESP, want %x", resp.ReqID, id)
		}
		if len(resp.Message) > 0 {
			t.Fatalf("non-empty message %x in TALKRESP", resp.Message)
		}
	default:
		t.Fatal("expected TALKRESP, got", resp.Name())
	}

	
	resp = conn.reqresp(l1, &v5wire.TalkRequest{Protocol: "test-protocol"})
	switch resp := resp.(type) {
	case *v5wire.TalkResponse:
		if len(resp.ReqID) > 0 {
			t.Fatalf("wrong request ID %x in TALKRESP, want empty byte array", resp.ReqID)
		}
		if len(resp.Message) > 0 {
			t.Fatalf("non-empty message %x in TALKRESP", resp.Message)
		}
	default:
		t.Fatal("expected TALKRESP, got", resp.Name())
	}
}


func (s *Suite) TestFindnodeZeroDistance(t *utesting.T) {
	conn, l1 := s.listen1(t)
	defer conn.close()

	nodes, err := conn.findnode(l1, []uint{0})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("remote returned more than one node for FINDNODE [0]")
	}
	if nodes[0].ID() != conn.remote.ID() {
		t.Errorf("ID of response node is %v, want %v", nodes[0].ID(), conn.remote.ID())
	}
}



func (s *Suite) TestFindnodeResults(t *utesting.T) {
	
	nodes := make([]*bystander, 5)
	added := make(chan enode.ID, len(nodes))
	for i := range nodes {
		nodes[i] = newBystander(t, s, added)
		defer nodes[i].close()
	}

	
	timeout := 60 * time.Second
	timeoutCh := time.After(timeout)
	for count := 0; count < len(nodes); {
		select {
		case id := <-added:
			t.Logf("bystander node %v added to remote table", id)
			count++
		case <-timeoutCh:
			t.Errorf("remote added %d bystander nodes in %v, need %d to continue", count, timeout, len(nodes))
			t.Logf("this can happen if the node has a non-empty table from previous runs")
			return
		}
	}
	t.Logf("all %d bystander nodes were added", len(nodes))

	
	var dists []uint
	expect := make(map[enode.ID]*enode.Node)
	for _, bn := range nodes {
		n := bn.conn.localNode.Node()
		expect[n.ID()] = n
		d := uint(enode.LogDist(n.ID(), s.Dest.ID()))
		if !containsUint(dists, d) {
			dists = append(dists, d)
		}
	}

	
	conn, l1 := s.listen1(t)
	defer conn.close()
	foundNodes, err := conn.findnode(l1, dists)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("remote returned %d nodes for distance list %v", len(foundNodes), dists)
	for _, n := range foundNodes {
		delete(expect, n.ID())
	}
	if len(expect) > 0 {
		t.Errorf("missing %d nodes in FINDNODE result", len(expect))
		t.Logf("this can happen if the test is run multiple times in quick succession")
		t.Logf("and the remote node hasn't removed dead nodes from previous runs yet")
	} else {
		t.Logf("all %d expected nodes were returned", len(nodes))
	}
}


type bystander struct {
	dest *enode.Node
	conn *conn
	l    net.PacketConn

	addedCh chan enode.ID
	done    sync.WaitGroup
}

func newBystander(t *utesting.T, s *Suite, added chan enode.ID) *bystander {
	conn, l := s.listen1(t)
	conn.setEndpoint(l) 
	bn := &bystander{
		conn:    conn,
		l:       l,
		dest:    s.Dest,
		addedCh: added,
	}
	bn.done.Add(1)
	go bn.loop()
	return bn
}


func (bn *bystander) id() enode.ID {
	return bn.conn.localNode.ID()
}


func (bn *bystander) close() {
	bn.conn.close()
	bn.done.Wait()
}


func (bn *bystander) loop() {
	defer bn.done.Done()

	var (
		lastPing time.Time
		wasAdded bool
	)
	for {
		
		if !wasAdded && time.Since(lastPing) > 10*time.Second {
			bn.conn.reqresp(bn.l, &v5wire.Ping{
				ReqID:  bn.conn.nextReqID(),
				ENRSeq: bn.dest.Seq(),
			})
			lastPing = time.Now()
		}
		
		switch p := bn.conn.read(bn.l).(type) {
		case *v5wire.Ping:
			bn.conn.write(bn.l, &v5wire.Pong{
				ReqID:  p.ReqID,
				ENRSeq: bn.conn.localNode.Seq(),
				ToIP:   bn.dest.IP(),
				ToPort: uint16(bn.dest.UDP()),
			}, nil)
			wasAdded = true
			bn.notifyAdded()
		case *v5wire.Findnode:
			bn.conn.write(bn.l, &v5wire.Nodes{ReqID: p.ReqID, Total: 1}, nil)
			wasAdded = true
			bn.notifyAdded()
		case *v5wire.TalkRequest:
			bn.conn.write(bn.l, &v5wire.TalkResponse{ReqID: p.ReqID}, nil)
		case *readError:
			if !netutil.IsTemporaryError(p.err) {
				bn.conn.logf("shutting down: %v", p.err)
				return
			}
		}
	}
}

func (bn *bystander) notifyAdded() {
	if bn.addedCh != nil {
		bn.addedCh <- bn.id()
		bn.addedCh = nil
	}
}
