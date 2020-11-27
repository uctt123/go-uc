















package discover

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover/v5wire"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)

const (
	lookupRequestLimit      = 3  
	findnodeResultLimit     = 16 
	totalNodesResponseLimit = 5  
	nodesResponseItemLimit  = 3  

	respTimeoutV5 = 700 * time.Millisecond
)





type codecV5 interface {
	
	Encode(enode.ID, string, v5wire.Packet, *v5wire.Whoareyou) ([]byte, v5wire.Nonce, error)

	
	
	Decode([]byte, string) (enode.ID, *enode.Node, v5wire.Packet, error)
}


type UDPv5 struct {
	
	conn         UDPConn
	tab          *Table
	netrestrict  *netutil.Netlist
	priv         *ecdsa.PrivateKey
	localNode    *enode.LocalNode
	db           *enode.DB
	log          log.Logger
	clock        mclock.Clock
	validSchemes enr.IdentityScheme

	
	trlock     sync.Mutex
	trhandlers map[string]func([]byte) []byte

	
	packetInCh    chan ReadPacket
	readNextCh    chan struct{}
	callCh        chan *callV5
	callDoneCh    chan *callV5
	respTimeoutCh chan *callTimeout

	
	codec            codecV5
	activeCallByNode map[enode.ID]*callV5
	activeCallByAuth map[v5wire.Nonce]*callV5
	callQueue        map[enode.ID][]*callV5

	
	closeOnce      sync.Once
	closeCtx       context.Context
	cancelCloseCtx context.CancelFunc
	wg             sync.WaitGroup
}


type callV5 struct {
	node         *enode.Node
	packet       v5wire.Packet
	responseType byte 
	reqid        []byte
	ch           chan v5wire.Packet 
	err          chan error         

	
	nonce          v5wire.Nonce      
	handshakeCount int               
	challenge      *v5wire.Whoareyou 
	timeout        mclock.Timer
}


type callTimeout struct {
	c     *callV5
	timer mclock.Timer
}


func ListenV5(conn UDPConn, ln *enode.LocalNode, cfg Config) (*UDPv5, error) {
	t, err := newUDPv5(conn, ln, cfg)
	if err != nil {
		return nil, err
	}
	go t.tab.loop()
	t.wg.Add(2)
	go t.readLoop()
	go t.dispatch()
	return t, nil
}


func newUDPv5(conn UDPConn, ln *enode.LocalNode, cfg Config) (*UDPv5, error) {
	closeCtx, cancelCloseCtx := context.WithCancel(context.Background())
	cfg = cfg.withDefaults()
	t := &UDPv5{
		
		conn:         conn,
		localNode:    ln,
		db:           ln.Database(),
		netrestrict:  cfg.NetRestrict,
		priv:         cfg.PrivateKey,
		log:          cfg.Log,
		validSchemes: cfg.ValidSchemes,
		clock:        cfg.Clock,
		trhandlers:   make(map[string]func([]byte) []byte),
		
		packetInCh:    make(chan ReadPacket, 1),
		readNextCh:    make(chan struct{}, 1),
		callCh:        make(chan *callV5),
		callDoneCh:    make(chan *callV5),
		respTimeoutCh: make(chan *callTimeout),
		
		codec:            v5wire.NewCodec(ln, cfg.PrivateKey, cfg.Clock),
		activeCallByNode: make(map[enode.ID]*callV5),
		activeCallByAuth: make(map[v5wire.Nonce]*callV5),
		callQueue:        make(map[enode.ID][]*callV5),
		
		closeCtx:       closeCtx,
		cancelCloseCtx: cancelCloseCtx,
	}
	tab, err := newTable(t, t.db, cfg.Bootnodes, cfg.Log)
	if err != nil {
		return nil, err
	}
	t.tab = tab
	return t, nil
}


func (t *UDPv5) Self() *enode.Node {
	return t.localNode.Node()
}


func (t *UDPv5) Close() {
	t.closeOnce.Do(func() {
		t.cancelCloseCtx()
		t.conn.Close()
		t.wg.Wait()
		t.tab.close()
	})
}


func (t *UDPv5) Ping(n *enode.Node) error {
	_, err := t.ping(n)
	return err
}



func (t *UDPv5) Resolve(n *enode.Node) *enode.Node {
	if intable := t.tab.getNode(n.ID()); intable != nil && intable.Seq() > n.Seq() {
		n = intable
	}
	
	if resp, err := t.RequestENR(n); err == nil {
		return resp
	}
	
	result := t.Lookup(n.ID())
	for _, rn := range result {
		if rn.ID() == n.ID() && rn.Seq() > n.Seq() {
			return rn
		}
	}
	return n
}


func (t *UDPv5) AllNodes() []*enode.Node {
	t.tab.mutex.Lock()
	defer t.tab.mutex.Unlock()
	nodes := make([]*enode.Node, 0)

	for _, b := range &t.tab.buckets {
		for _, n := range b.entries {
			nodes = append(nodes, unwrapNode(n))
		}
	}
	return nodes
}



func (t *UDPv5) LocalNode() *enode.LocalNode {
	return t.localNode
}




func (t *UDPv5) RegisterTalkHandler(protocol string, handler func([]byte) []byte) {
	t.trlock.Lock()
	defer t.trlock.Unlock()
	t.trhandlers[protocol] = handler
}


func (t *UDPv5) TalkRequest(n *enode.Node, protocol string, request []byte) ([]byte, error) {
	req := &v5wire.TalkRequest{Protocol: protocol, Message: request}
	resp := t.call(n, v5wire.TalkResponseMsg, req)
	defer t.callDone(resp)
	select {
	case respMsg := <-resp.ch:
		return respMsg.(*v5wire.TalkResponse).Message, nil
	case err := <-resp.err:
		return nil, err
	}
}


func (t *UDPv5) RandomNodes() enode.Iterator {
	if t.tab.len() == 0 {
		
		
		<-t.tab.refresh()
	}

	return newLookupIterator(t.closeCtx, t.newRandomLookup)
}



func (t *UDPv5) Lookup(target enode.ID) []*enode.Node {
	return t.newLookup(t.closeCtx, target).run()
}



func (t *UDPv5) lookupRandom() []*enode.Node {
	return t.newRandomLookup(t.closeCtx).run()
}



func (t *UDPv5) lookupSelf() []*enode.Node {
	return t.newLookup(t.closeCtx, t.Self().ID()).run()
}

func (t *UDPv5) newRandomLookup(ctx context.Context) *lookup {
	var target enode.ID
	crand.Read(target[:])
	return t.newLookup(ctx, target)
}

func (t *UDPv5) newLookup(ctx context.Context, target enode.ID) *lookup {
	return newLookup(ctx, t.tab, target, func(n *node) ([]*node, error) {
		return t.lookupWorker(n, target)
	})
}


func (t *UDPv5) lookupWorker(destNode *node, target enode.ID) ([]*node, error) {
	var (
		dists = lookupDistances(target, destNode.ID())
		nodes = nodesByDistance{target: target}
		err   error
	)
	var r []*enode.Node
	r, err = t.findnode(unwrapNode(destNode), dists)
	if err == errClosed {
		return nil, err
	}
	for _, n := range r {
		if n.ID() != t.Self().ID() {
			nodes.push(wrapNode(n), findnodeResultLimit)
		}
	}
	return nodes.entries, err
}




func lookupDistances(target, dest enode.ID) (dists []uint) {
	td := enode.LogDist(target, dest)
	dists = append(dists, uint(td))
	for i := 1; len(dists) < lookupRequestLimit; i++ {
		if td+i < 256 {
			dists = append(dists, uint(td+i))
		}
		if td-i > 0 {
			dists = append(dists, uint(td-i))
		}
	}
	return dists
}


func (t *UDPv5) ping(n *enode.Node) (uint64, error) {
	req := &v5wire.Ping{ENRSeq: t.localNode.Node().Seq()}
	resp := t.call(n, v5wire.PongMsg, req)
	defer t.callDone(resp)

	select {
	case pong := <-resp.ch:
		return pong.(*v5wire.Pong).ENRSeq, nil
	case err := <-resp.err:
		return 0, err
	}
}


func (t *UDPv5) RequestENR(n *enode.Node) (*enode.Node, error) {
	nodes, err := t.findnode(n, []uint{0})
	if err != nil {
		return nil, err
	}
	if len(nodes) != 1 {
		return nil, fmt.Errorf("%d nodes in response for distance zero", len(nodes))
	}
	return nodes[0], nil
}


func (t *UDPv5) findnode(n *enode.Node, distances []uint) ([]*enode.Node, error) {
	resp := t.call(n, v5wire.NodesMsg, &v5wire.Findnode{Distances: distances})
	return t.waitForNodes(resp, distances)
}


func (t *UDPv5) waitForNodes(c *callV5, distances []uint) ([]*enode.Node, error) {
	defer t.callDone(c)

	var (
		nodes           []*enode.Node
		seen            = make(map[enode.ID]struct{})
		received, total = 0, -1
	)
	for {
		select {
		case responseP := <-c.ch:
			response := responseP.(*v5wire.Nodes)
			for _, record := range response.Nodes {
				node, err := t.verifyResponseNode(c, record, distances, seen)
				if err != nil {
					t.log.Debug("Invalid record in "+response.Name(), "id", c.node.ID(), "err", err)
					continue
				}
				nodes = append(nodes, node)
			}
			if total == -1 {
				total = min(int(response.Total), totalNodesResponseLimit)
			}
			if received++; received == total {
				return nodes, nil
			}
		case err := <-c.err:
			return nodes, err
		}
	}
}


func (t *UDPv5) verifyResponseNode(c *callV5, r *enr.Record, distances []uint, seen map[enode.ID]struct{}) (*enode.Node, error) {
	node, err := enode.New(t.validSchemes, r)
	if err != nil {
		return nil, err
	}
	if err := netutil.CheckRelayIP(c.node.IP(), node.IP()); err != nil {
		return nil, err
	}
	if c.node.UDP() <= 1024 {
		return nil, errLowPort
	}
	if distances != nil {
		nd := enode.LogDist(c.node.ID(), node.ID())
		if !containsUint(uint(nd), distances) {
			return nil, errors.New("does not match any requested distance")
		}
	}
	if _, ok := seen[node.ID()]; ok {
		return nil, fmt.Errorf("duplicate record")
	}
	seen[node.ID()] = struct{}{}
	return node, nil
}

func containsUint(x uint, xs []uint) bool {
	for _, v := range xs {
		if x == v {
			return true
		}
	}
	return false
}



func (t *UDPv5) call(node *enode.Node, responseType byte, packet v5wire.Packet) *callV5 {
	c := &callV5{
		node:         node,
		packet:       packet,
		responseType: responseType,
		reqid:        make([]byte, 8),
		ch:           make(chan v5wire.Packet, 1),
		err:          make(chan error, 1),
	}
	
	crand.Read(c.reqid)
	packet.SetRequestID(c.reqid)
	
	select {
	case t.callCh <- c:
	case <-t.closeCtx.Done():
		c.err <- errClosed
	}
	return c
}


func (t *UDPv5) callDone(c *callV5) {
	select {
	case t.callDoneCh <- c:
	case <-t.closeCtx.Done():
	}
}












func (t *UDPv5) dispatch() {
	defer t.wg.Done()

	
	t.readNextCh <- struct{}{}

	for {
		select {
		case c := <-t.callCh:
			id := c.node.ID()
			t.callQueue[id] = append(t.callQueue[id], c)
			t.sendNextCall(id)

		case ct := <-t.respTimeoutCh:
			active := t.activeCallByNode[ct.c.node.ID()]
			if ct.c == active && ct.timer == active.timeout {
				ct.c.err <- errTimeout
			}

		case c := <-t.callDoneCh:
			id := c.node.ID()
			active := t.activeCallByNode[id]
			if active != c {
				panic("BUG: callDone for inactive call")
			}
			c.timeout.Stop()
			delete(t.activeCallByAuth, c.nonce)
			delete(t.activeCallByNode, id)
			t.sendNextCall(id)

		case p := <-t.packetInCh:
			t.handlePacket(p.Data, p.Addr)
			
			t.readNextCh <- struct{}{}

		case <-t.closeCtx.Done():
			close(t.readNextCh)
			for id, queue := range t.callQueue {
				for _, c := range queue {
					c.err <- errClosed
				}
				delete(t.callQueue, id)
			}
			for id, c := range t.activeCallByNode {
				c.err <- errClosed
				delete(t.activeCallByNode, id)
				delete(t.activeCallByAuth, c.nonce)
			}
			return
		}
	}
}


func (t *UDPv5) startResponseTimeout(c *callV5) {
	if c.timeout != nil {
		c.timeout.Stop()
	}
	var (
		timer mclock.Timer
		done  = make(chan struct{})
	)
	timer = t.clock.AfterFunc(respTimeoutV5, func() {
		<-done
		select {
		case t.respTimeoutCh <- &callTimeout{c, timer}:
		case <-t.closeCtx.Done():
		}
	})
	c.timeout = timer
	close(done)
}


func (t *UDPv5) sendNextCall(id enode.ID) {
	queue := t.callQueue[id]
	if len(queue) == 0 || t.activeCallByNode[id] != nil {
		return
	}
	t.activeCallByNode[id] = queue[0]
	t.sendCall(t.activeCallByNode[id])
	if len(queue) == 1 {
		delete(t.callQueue, id)
	} else {
		copy(queue, queue[1:])
		t.callQueue[id] = queue[:len(queue)-1]
	}
}



func (t *UDPv5) sendCall(c *callV5) {
	
	
	if c.nonce != (v5wire.Nonce{}) {
		delete(t.activeCallByAuth, c.nonce)
	}

	addr := &net.UDPAddr{IP: c.node.IP(), Port: c.node.UDP()}
	newNonce, _ := t.send(c.node.ID(), addr, c.packet, c.challenge)
	c.nonce = newNonce
	t.activeCallByAuth[newNonce] = c
	t.startResponseTimeout(c)
}



func (t *UDPv5) sendResponse(toID enode.ID, toAddr *net.UDPAddr, packet v5wire.Packet) error {
	_, err := t.send(toID, toAddr, packet, nil)
	return err
}


func (t *UDPv5) send(toID enode.ID, toAddr *net.UDPAddr, packet v5wire.Packet, c *v5wire.Whoareyou) (v5wire.Nonce, error) {
	addr := toAddr.String()
	enc, nonce, err := t.codec.Encode(toID, addr, packet, c)
	if err != nil {
		t.log.Warn(">> "+packet.Name(), "id", toID, "addr", addr, "err", err)
		return nonce, err
	}
	_, err = t.conn.WriteToUDP(enc, toAddr)
	t.log.Trace(">> "+packet.Name(), "id", toID, "addr", addr)
	return nonce, err
}


func (t *UDPv5) readLoop() {
	defer t.wg.Done()

	buf := make([]byte, maxPacketSize)
	for range t.readNextCh {
		nbytes, from, err := t.conn.ReadFromUDP(buf)
		if netutil.IsTemporaryError(err) {
			
			t.log.Debug("Temporary UDP read error", "err", err)
			continue
		} else if err != nil {
			
			if err != io.EOF {
				t.log.Debug("UDP read error", "err", err)
			}
			return
		}
		t.dispatchReadPacket(from, buf[:nbytes])
	}
}


func (t *UDPv5) dispatchReadPacket(from *net.UDPAddr, content []byte) bool {
	select {
	case t.packetInCh <- ReadPacket{content, from}:
		return true
	case <-t.closeCtx.Done():
		return false
	}
}


func (t *UDPv5) handlePacket(rawpacket []byte, fromAddr *net.UDPAddr) error {
	addr := fromAddr.String()
	fromID, fromNode, packet, err := t.codec.Decode(rawpacket, addr)
	if err != nil {
		t.log.Debug("Bad discv5 packet", "id", fromID, "addr", addr, "err", err)
		return err
	}
	if fromNode != nil {
		
		t.tab.addSeenNode(wrapNode(fromNode))
	}
	if packet.Kind() != v5wire.WhoareyouPacket {
		
		t.log.Trace("<< "+packet.Name(), "id", fromID, "addr", addr)
	}
	t.handle(packet, fromID, fromAddr)
	return nil
}


func (t *UDPv5) handleCallResponse(fromID enode.ID, fromAddr *net.UDPAddr, p v5wire.Packet) bool {
	ac := t.activeCallByNode[fromID]
	if ac == nil || !bytes.Equal(p.RequestID(), ac.reqid) {
		t.log.Debug(fmt.Sprintf("Unsolicited/late %s response", p.Name()), "id", fromID, "addr", fromAddr)
		return false
	}
	if !fromAddr.IP.Equal(ac.node.IP()) || fromAddr.Port != ac.node.UDP() {
		t.log.Debug(fmt.Sprintf("%s from wrong endpoint", p.Name()), "id", fromID, "addr", fromAddr)
		return false
	}
	if p.Kind() != ac.responseType {
		t.log.Debug(fmt.Sprintf("Wrong discv5 response type %s", p.Name()), "id", fromID, "addr", fromAddr)
		return false
	}
	t.startResponseTimeout(ac)
	ac.ch <- p
	return true
}


func (t *UDPv5) getNode(id enode.ID) *enode.Node {
	if n := t.tab.getNode(id); n != nil {
		return n
	}
	if n := t.localNode.Database().Node(id); n != nil {
		return n
	}
	return nil
}


func (t *UDPv5) handle(p v5wire.Packet, fromID enode.ID, fromAddr *net.UDPAddr) {
	switch p := p.(type) {
	case *v5wire.Unknown:
		t.handleUnknown(p, fromID, fromAddr)
	case *v5wire.Whoareyou:
		t.handleWhoareyou(p, fromID, fromAddr)
	case *v5wire.Ping:
		t.handlePing(p, fromID, fromAddr)
	case *v5wire.Pong:
		if t.handleCallResponse(fromID, fromAddr, p) {
			t.localNode.UDPEndpointStatement(fromAddr, &net.UDPAddr{IP: p.ToIP, Port: int(p.ToPort)})
		}
	case *v5wire.Findnode:
		t.handleFindnode(p, fromID, fromAddr)
	case *v5wire.Nodes:
		t.handleCallResponse(fromID, fromAddr, p)
	case *v5wire.TalkRequest:
		t.handleTalkRequest(p, fromID, fromAddr)
	case *v5wire.TalkResponse:
		t.handleCallResponse(fromID, fromAddr, p)
	}
}


func (t *UDPv5) handleUnknown(p *v5wire.Unknown, fromID enode.ID, fromAddr *net.UDPAddr) {
	challenge := &v5wire.Whoareyou{Nonce: p.Nonce}
	crand.Read(challenge.IDNonce[:])
	if n := t.getNode(fromID); n != nil {
		challenge.Node = n
		challenge.RecordSeq = n.Seq()
	}
	t.sendResponse(fromID, fromAddr, challenge)
}

var (
	errChallengeNoCall = errors.New("no matching call")
	errChallengeTwice  = errors.New("second handshake")
)


func (t *UDPv5) handleWhoareyou(p *v5wire.Whoareyou, fromID enode.ID, fromAddr *net.UDPAddr) {
	c, err := t.matchWithCall(fromID, p.Nonce)
	if err != nil {
		t.log.Debug("Invalid "+p.Name(), "addr", fromAddr, "err", err)
		return
	}

	
	t.log.Trace("<< "+p.Name(), "id", c.node.ID(), "addr", fromAddr)
	c.handshakeCount++
	c.challenge = p
	p.Node = c.node
	t.sendCall(c)
}


func (t *UDPv5) matchWithCall(fromID enode.ID, nonce v5wire.Nonce) (*callV5, error) {
	c := t.activeCallByAuth[nonce]
	if c == nil {
		return nil, errChallengeNoCall
	}
	if c.handshakeCount > 0 {
		return nil, errChallengeTwice
	}
	return c, nil
}


func (t *UDPv5) handlePing(p *v5wire.Ping, fromID enode.ID, fromAddr *net.UDPAddr) {
	t.sendResponse(fromID, fromAddr, &v5wire.Pong{
		ReqID:  p.ReqID,
		ToIP:   fromAddr.IP,
		ToPort: uint16(fromAddr.Port),
		ENRSeq: t.localNode.Node().Seq(),
	})
}


func (t *UDPv5) handleFindnode(p *v5wire.Findnode, fromID enode.ID, fromAddr *net.UDPAddr) {
	nodes := t.collectTableNodes(fromAddr.IP, p.Distances, findnodeResultLimit)
	for _, resp := range packNodes(p.ReqID, nodes) {
		t.sendResponse(fromID, fromAddr, resp)
	}
}


func (t *UDPv5) collectTableNodes(rip net.IP, distances []uint, limit int) []*enode.Node {
	var nodes []*enode.Node
	var processed = make(map[uint]struct{})
	for _, dist := range distances {
		
		_, seen := processed[dist]
		if seen || dist > 256 {
			continue
		}

		
		var bn []*enode.Node
		if dist == 0 {
			bn = []*enode.Node{t.Self()}
		} else if dist <= 256 {
			t.tab.mutex.Lock()
			bn = unwrapNodes(t.tab.bucketAtDistance(int(dist)).entries)
			t.tab.mutex.Unlock()
		}
		processed[dist] = struct{}{}

		
		for _, n := range bn {
			
			if netutil.CheckRelayIP(rip, n.IP()) != nil {
				continue
			}
			nodes = append(nodes, n)
			if len(nodes) >= limit {
				return nodes
			}
		}
	}
	return nodes
}


func packNodes(reqid []byte, nodes []*enode.Node) []*v5wire.Nodes {
	if len(nodes) == 0 {
		return []*v5wire.Nodes{{ReqID: reqid, Total: 1}}
	}

	total := uint8(math.Ceil(float64(len(nodes)) / 3))
	var resp []*v5wire.Nodes
	for len(nodes) > 0 {
		p := &v5wire.Nodes{ReqID: reqid, Total: total}
		items := min(nodesResponseItemLimit, len(nodes))
		for i := 0; i < items; i++ {
			p.Nodes = append(p.Nodes, nodes[i].Record())
		}
		nodes = nodes[items:]
		resp = append(resp, p)
	}
	return resp
}


func (t *UDPv5) handleTalkRequest(p *v5wire.TalkRequest, fromID enode.ID, fromAddr *net.UDPAddr) {
	t.trlock.Lock()
	handler := t.trhandlers[p.Protocol]
	t.trlock.Unlock()

	var response []byte
	if handler != nil {
		response = handler(p.Message)
	}
	resp := &v5wire.TalkResponse{ReqID: p.ReqID, Message: response}
	t.sendResponse(fromID, fromAddr, resp)
}
