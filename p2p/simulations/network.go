















package simulations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/simulations/adapters"
)

var DialBanTimeout = 200 * time.Millisecond


type NetworkConfig struct {
	ID             string `json:"id"`
	DefaultService string `json:"default_service,omitempty"`
}









type Network struct {
	NetworkConfig

	Nodes   []*Node `json:"nodes"`
	nodeMap map[enode.ID]int

	
	propertyMap map[string][]int

	Conns   []*Conn `json:"conns"`
	connMap map[string]int

	nodeAdapter adapters.NodeAdapter
	events      event.Feed
	lock        sync.RWMutex
	quitc       chan struct{}
}


func NewNetwork(nodeAdapter adapters.NodeAdapter, conf *NetworkConfig) *Network {
	return &Network{
		NetworkConfig: *conf,
		nodeAdapter:   nodeAdapter,
		nodeMap:       make(map[enode.ID]int),
		propertyMap:   make(map[string][]int),
		connMap:       make(map[string]int),
		quitc:         make(chan struct{}),
	}
}


func (net *Network) Events() *event.Feed {
	return &net.events
}



func (net *Network) NewNodeWithConfig(conf *adapters.NodeConfig) (*Node, error) {
	net.lock.Lock()
	defer net.lock.Unlock()

	if conf.Reachable == nil {
		conf.Reachable = func(otherID enode.ID) bool {
			_, err := net.InitConn(conf.ID, otherID)
			if err != nil && bytes.Compare(conf.ID.Bytes(), otherID.Bytes()) < 0 {
				return false
			}
			return true
		}
	}

	
	if node := net.getNode(conf.ID); node != nil {
		return nil, fmt.Errorf("node with ID %q already exists", conf.ID)
	}
	if node := net.getNodeByName(conf.Name); node != nil {
		return nil, fmt.Errorf("node with name %q already exists", conf.Name)
	}

	
	if len(conf.Lifecycles) == 0 {
		conf.Lifecycles = []string{net.DefaultService}
	}

	
	adapterNode, err := net.nodeAdapter.NewNode(conf)
	if err != nil {
		return nil, err
	}
	node := newNode(adapterNode, conf, false)
	log.Trace("Node created", "id", conf.ID)

	nodeIndex := len(net.Nodes)
	net.nodeMap[conf.ID] = nodeIndex
	net.Nodes = append(net.Nodes, node)

	
	for _, property := range conf.Properties {
		net.propertyMap[property] = append(net.propertyMap[property], nodeIndex)
	}

	
	net.events.Send(ControlEvent(node))

	return node, nil
}


func (net *Network) Config() *NetworkConfig {
	return &net.NetworkConfig
}


func (net *Network) StartAll() error {
	for _, node := range net.Nodes {
		if node.Up() {
			continue
		}
		if err := net.Start(node.ID()); err != nil {
			return err
		}
	}
	return nil
}


func (net *Network) StopAll() error {
	for _, node := range net.Nodes {
		if !node.Up() {
			continue
		}
		if err := net.Stop(node.ID()); err != nil {
			return err
		}
	}
	return nil
}


func (net *Network) Start(id enode.ID) error {
	return net.startWithSnapshots(id, nil)
}



func (net *Network) startWithSnapshots(id enode.ID, snapshots map[string][]byte) error {
	net.lock.Lock()
	defer net.lock.Unlock()

	node := net.getNode(id)
	if node == nil {
		return fmt.Errorf("node %v does not exist", id)
	}
	if node.Up() {
		return fmt.Errorf("node %v already up", id)
	}
	log.Trace("Starting node", "id", id, "adapter", net.nodeAdapter.Name())
	if err := node.Start(snapshots); err != nil {
		log.Warn("Node startup failed", "id", id, "err", err)
		return err
	}
	node.SetUp(true)
	log.Info("Started node", "id", id)
	ev := NewEvent(node)
	net.events.Send(ev)

	
	client, err := node.Client()
	if err != nil {
		return fmt.Errorf("error getting rpc client  for node %v: %s", id, err)
	}
	events := make(chan *p2p.PeerEvent)
	sub, err := client.Subscribe(context.Background(), "admin", events, "peerEvents")
	if err != nil {
		return fmt.Errorf("error getting peer events for node %v: %s", id, err)
	}
	go net.watchPeerEvents(id, events, sub)
	return nil
}



func (net *Network) watchPeerEvents(id enode.ID, events chan *p2p.PeerEvent, sub event.Subscription) {
	defer func() {
		sub.Unsubscribe()

		
		net.lock.Lock()
		defer net.lock.Unlock()

		node := net.getNode(id)
		if node == nil {
			return
		}
		node.SetUp(false)
		ev := NewEvent(node)
		net.events.Send(ev)
	}()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			peer := event.Peer
			switch event.Type {

			case p2p.PeerEventTypeAdd:
				net.DidConnect(id, peer)

			case p2p.PeerEventTypeDrop:
				net.DidDisconnect(id, peer)

			case p2p.PeerEventTypeMsgSend:
				net.DidSend(id, peer, event.Protocol, *event.MsgCode)

			case p2p.PeerEventTypeMsgRecv:
				net.DidReceive(peer, id, event.Protocol, *event.MsgCode)

			}

		case err := <-sub.Err():
			if err != nil {
				log.Error("Error in peer event subscription", "id", id, "err", err)
			}
			return
		}
	}
}


func (net *Network) Stop(id enode.ID) error {
	
	
	
	

	var err error

	node, err := func() (*Node, error) {
		net.lock.Lock()
		defer net.lock.Unlock()

		node := net.getNode(id)
		if node == nil {
			return nil, fmt.Errorf("node %v does not exist", id)
		}
		if !node.Up() {
			return nil, fmt.Errorf("node %v already down", id)
		}
		node.SetUp(false)
		return node, nil
	}()
	if err != nil {
		return err
	}

	err = node.Stop() 

	net.lock.Lock()
	defer net.lock.Unlock()

	if err != nil {
		node.SetUp(true)
		return err
	}
	log.Info("Stopped node", "id", id, "err", err)
	ev := ControlEvent(node)
	net.events.Send(ev)
	return nil
}



func (net *Network) Connect(oneID, otherID enode.ID) error {
	net.lock.Lock()
	defer net.lock.Unlock()
	return net.connect(oneID, otherID)
}

func (net *Network) connect(oneID, otherID enode.ID) error {
	log.Debug("Connecting nodes with addPeer", "id", oneID, "other", otherID)
	conn, err := net.initConn(oneID, otherID)
	if err != nil {
		return err
	}
	client, err := conn.one.Client()
	if err != nil {
		return err
	}
	net.events.Send(ControlEvent(conn))
	return client.Call(nil, "admin_addPeer", string(conn.other.Addr()))
}



func (net *Network) Disconnect(oneID, otherID enode.ID) error {
	conn := net.GetConn(oneID, otherID)
	if conn == nil {
		return fmt.Errorf("connection between %v and %v does not exist", oneID, otherID)
	}
	if !conn.Up {
		return fmt.Errorf("%v and %v already disconnected", oneID, otherID)
	}
	client, err := conn.one.Client()
	if err != nil {
		return err
	}
	net.events.Send(ControlEvent(conn))
	return client.Call(nil, "admin_removePeer", string(conn.other.Addr()))
}


func (net *Network) DidConnect(one, other enode.ID) error {
	net.lock.Lock()
	defer net.lock.Unlock()
	conn, err := net.getOrCreateConn(one, other)
	if err != nil {
		return fmt.Errorf("connection between %v and %v does not exist", one, other)
	}
	if conn.Up {
		return fmt.Errorf("%v and %v already connected", one, other)
	}
	conn.Up = true
	net.events.Send(NewEvent(conn))
	return nil
}



func (net *Network) DidDisconnect(one, other enode.ID) error {
	net.lock.Lock()
	defer net.lock.Unlock()
	conn := net.getConn(one, other)
	if conn == nil {
		return fmt.Errorf("connection between %v and %v does not exist", one, other)
	}
	if !conn.Up {
		return fmt.Errorf("%v and %v already disconnected", one, other)
	}
	conn.Up = false
	conn.initiated = time.Now().Add(-DialBanTimeout)
	net.events.Send(NewEvent(conn))
	return nil
}


func (net *Network) DidSend(sender, receiver enode.ID, proto string, code uint64) error {
	msg := &Msg{
		One:      sender,
		Other:    receiver,
		Protocol: proto,
		Code:     code,
		Received: false,
	}
	net.events.Send(NewEvent(msg))
	return nil
}


func (net *Network) DidReceive(sender, receiver enode.ID, proto string, code uint64) error {
	msg := &Msg{
		One:      sender,
		Other:    receiver,
		Protocol: proto,
		Code:     code,
		Received: true,
	}
	net.events.Send(NewEvent(msg))
	return nil
}



func (net *Network) GetNode(id enode.ID) *Node {
	net.lock.RLock()
	defer net.lock.RUnlock()
	return net.getNode(id)
}

func (net *Network) getNode(id enode.ID) *Node {
	i, found := net.nodeMap[id]
	if !found {
		return nil
	}
	return net.Nodes[i]
}



func (net *Network) GetNodeByName(name string) *Node {
	net.lock.RLock()
	defer net.lock.RUnlock()
	return net.getNodeByName(name)
}

func (net *Network) getNodeByName(name string) *Node {
	for _, node := range net.Nodes {
		if node.Config.Name == name {
			return node
		}
	}
	return nil
}



func (net *Network) GetNodeIDs(excludeIDs ...enode.ID) []enode.ID {
	net.lock.RLock()
	defer net.lock.RUnlock()

	return net.getNodeIDs(excludeIDs)
}

func (net *Network) getNodeIDs(excludeIDs []enode.ID) []enode.ID {
	
	nodeIDs := make([]enode.ID, 0, len(net.nodeMap))
	for id := range net.nodeMap {
		nodeIDs = append(nodeIDs, id)
	}

	if len(excludeIDs) > 0 {
		
		return filterIDs(nodeIDs, excludeIDs)
	} else {
		return nodeIDs
	}
}



func (net *Network) GetNodes(excludeIDs ...enode.ID) []*Node {
	net.lock.RLock()
	defer net.lock.RUnlock()

	return net.getNodes(excludeIDs)
}

func (net *Network) getNodes(excludeIDs []enode.ID) []*Node {
	if len(excludeIDs) > 0 {
		nodeIDs := net.getNodeIDs(excludeIDs)
		return net.getNodesByID(nodeIDs)
	} else {
		return net.Nodes
	}
}



func (net *Network) GetNodesByID(nodeIDs []enode.ID) []*Node {
	net.lock.RLock()
	defer net.lock.RUnlock()

	return net.getNodesByID(nodeIDs)
}

func (net *Network) getNodesByID(nodeIDs []enode.ID) []*Node {
	nodes := make([]*Node, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		node := net.getNode(id)
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes
}


func (net *Network) GetNodesByProperty(property string) []*Node {
	net.lock.RLock()
	defer net.lock.RUnlock()

	return net.getNodesByProperty(property)
}

func (net *Network) getNodesByProperty(property string) []*Node {
	nodes := make([]*Node, 0, len(net.propertyMap[property]))
	for _, nodeIndex := range net.propertyMap[property] {
		nodes = append(nodes, net.Nodes[nodeIndex])
	}

	return nodes
}


func (net *Network) GetNodeIDsByProperty(property string) []enode.ID {
	net.lock.RLock()
	defer net.lock.RUnlock()

	return net.getNodeIDsByProperty(property)
}

func (net *Network) getNodeIDsByProperty(property string) []enode.ID {
	nodeIDs := make([]enode.ID, 0, len(net.propertyMap[property]))
	for _, nodeIndex := range net.propertyMap[property] {
		node := net.Nodes[nodeIndex]
		nodeIDs = append(nodeIDs, node.ID())
	}

	return nodeIDs
}


func (net *Network) GetRandomUpNode(excludeIDs ...enode.ID) *Node {
	net.lock.RLock()
	defer net.lock.RUnlock()
	return net.getRandomUpNode(excludeIDs...)
}


func (net *Network) getRandomUpNode(excludeIDs ...enode.ID) *Node {
	return net.getRandomNode(net.getUpNodeIDs(), excludeIDs)
}

func (net *Network) getUpNodeIDs() (ids []enode.ID) {
	for _, node := range net.Nodes {
		if node.Up() {
			ids = append(ids, node.ID())
		}
	}
	return ids
}


func (net *Network) GetRandomDownNode(excludeIDs ...enode.ID) *Node {
	net.lock.RLock()
	defer net.lock.RUnlock()
	return net.getRandomNode(net.getDownNodeIDs(), excludeIDs)
}

func (net *Network) getDownNodeIDs() (ids []enode.ID) {
	for _, node := range net.Nodes {
		if !node.Up() {
			ids = append(ids, node.ID())
		}
	}
	return ids
}


func (net *Network) GetRandomNode(excludeIDs ...enode.ID) *Node {
	net.lock.RLock()
	defer net.lock.RUnlock()
	return net.getRandomNode(net.getNodeIDs(nil), excludeIDs) 
}

func (net *Network) getRandomNode(ids []enode.ID, excludeIDs []enode.ID) *Node {
	filtered := filterIDs(ids, excludeIDs)

	l := len(filtered)
	if l == 0 {
		return nil
	}
	return net.getNode(filtered[rand.Intn(l)])
}

func filterIDs(ids []enode.ID, excludeIDs []enode.ID) []enode.ID {
	exclude := make(map[enode.ID]bool)
	for _, id := range excludeIDs {
		exclude[id] = true
	}
	var filtered []enode.ID
	for _, id := range ids {
		if _, found := exclude[id]; !found {
			filtered = append(filtered, id)
		}
	}
	return filtered
}



func (net *Network) GetConn(oneID, otherID enode.ID) *Conn {
	net.lock.RLock()
	defer net.lock.RUnlock()
	return net.getConn(oneID, otherID)
}



func (net *Network) GetOrCreateConn(oneID, otherID enode.ID) (*Conn, error) {
	net.lock.Lock()
	defer net.lock.Unlock()
	return net.getOrCreateConn(oneID, otherID)
}

func (net *Network) getOrCreateConn(oneID, otherID enode.ID) (*Conn, error) {
	if conn := net.getConn(oneID, otherID); conn != nil {
		return conn, nil
	}

	one := net.getNode(oneID)
	if one == nil {
		return nil, fmt.Errorf("node %v does not exist", oneID)
	}
	other := net.getNode(otherID)
	if other == nil {
		return nil, fmt.Errorf("node %v does not exist", otherID)
	}
	conn := &Conn{
		One:   oneID,
		Other: otherID,
		one:   one,
		other: other,
	}
	label := ConnLabel(oneID, otherID)
	net.connMap[label] = len(net.Conns)
	net.Conns = append(net.Conns, conn)
	return conn, nil
}

func (net *Network) getConn(oneID, otherID enode.ID) *Conn {
	label := ConnLabel(oneID, otherID)
	i, found := net.connMap[label]
	if !found {
		return nil
	}
	return net.Conns[i]
}









func (net *Network) InitConn(oneID, otherID enode.ID) (*Conn, error) {
	net.lock.Lock()
	defer net.lock.Unlock()
	return net.initConn(oneID, otherID)
}

func (net *Network) initConn(oneID, otherID enode.ID) (*Conn, error) {
	if oneID == otherID {
		return nil, fmt.Errorf("refusing to connect to self %v", oneID)
	}
	conn, err := net.getOrCreateConn(oneID, otherID)
	if err != nil {
		return nil, err
	}
	if conn.Up {
		return nil, fmt.Errorf("%v and %v already connected", oneID, otherID)
	}
	if time.Since(conn.initiated) < DialBanTimeout {
		return nil, fmt.Errorf("connection between %v and %v recently attempted", oneID, otherID)
	}

	err = conn.nodesUp()
	if err != nil {
		log.Trace("Nodes not up", "err", err)
		return nil, fmt.Errorf("nodes not up: %v", err)
	}
	log.Debug("Connection initiated", "id", oneID, "other", otherID)
	conn.initiated = time.Now()
	return conn, nil
}


func (net *Network) Shutdown() {
	for _, node := range net.Nodes {
		log.Debug("Stopping node", "id", node.ID())
		if err := node.Stop(); err != nil {
			log.Warn("Can't stop node", "id", node.ID(), "err", err)
		}
		
		if closer, ok := node.Node.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Warn("Can't close node", "id", node.ID(), "err", err)
			}
		}
	}
	close(net.quitc)
}



func (net *Network) Reset() {
	net.lock.Lock()
	defer net.lock.Unlock()

	
	net.connMap = make(map[string]int)
	net.nodeMap = make(map[enode.ID]int)
	net.propertyMap = make(map[string][]int)

	net.Nodes = nil
	net.Conns = nil
}



type Node struct {
	adapters.Node `json:"-"`

	
	Config *adapters.NodeConfig `json:"config"`

	
	up   bool
	upMu *sync.RWMutex
}

func newNode(an adapters.Node, ac *adapters.NodeConfig, up bool) *Node {
	return &Node{Node: an, Config: ac, up: up, upMu: new(sync.RWMutex)}
}

func (n *Node) copy() *Node {
	configCpy := *n.Config
	return newNode(n.Node, &configCpy, n.Up())
}


func (n *Node) Up() bool {
	n.upMu.RLock()
	defer n.upMu.RUnlock()
	return n.up
}


func (n *Node) SetUp(up bool) {
	n.upMu.Lock()
	defer n.upMu.Unlock()
	n.up = up
}


func (n *Node) ID() enode.ID {
	return n.Config.ID
}


func (n *Node) String() string {
	return fmt.Sprintf("Node %v", n.ID().TerminalString())
}


func (n *Node) NodeInfo() *p2p.NodeInfo {
	
	if n.Node == nil {
		return nil
	}
	info := n.Node.NodeInfo()
	info.Name = n.Config.Name
	return info
}



func (n *Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Info   *p2p.NodeInfo        `json:"info,omitempty"`
		Config *adapters.NodeConfig `json:"config,omitempty"`
		Up     bool                 `json:"up"`
	}{
		Info:   n.NodeInfo(),
		Config: n.Config,
		Up:     n.Up(),
	})
}



func (n *Node) UnmarshalJSON(raw []byte) error {
	
	
	var node struct {
		Config *adapters.NodeConfig `json:"config,omitempty"`
		Up     bool                 `json:"up"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		return err
	}
	*n = *newNode(nil, node.Config, node.Up)
	return nil
}


type Conn struct {
	
	One enode.ID `json:"one"`

	
	Other enode.ID `json:"other"`

	
	Up bool `json:"up"`
	
	initiated time.Time

	one   *Node
	other *Node
}


func (c *Conn) nodesUp() error {
	if !c.one.Up() {
		return fmt.Errorf("one %v is not up", c.One)
	}
	if !c.other.Up() {
		return fmt.Errorf("other %v is not up", c.Other)
	}
	return nil
}


func (c *Conn) String() string {
	return fmt.Sprintf("Conn %v->%v", c.One.TerminalString(), c.Other.TerminalString())
}


type Msg struct {
	One      enode.ID `json:"one"`
	Other    enode.ID `json:"other"`
	Protocol string   `json:"protocol"`
	Code     uint64   `json:"code"`
	Received bool     `json:"received"`
}


func (m *Msg) String() string {
	return fmt.Sprintf("Msg(%d) %v->%v", m.Code, m.One.TerminalString(), m.Other.TerminalString())
}




func ConnLabel(source, target enode.ID) string {
	var first, second enode.ID
	if bytes.Compare(source.Bytes(), target.Bytes()) > 0 {
		first = target
		second = source
	} else {
		first = source
		second = target
	}
	return fmt.Sprintf("%v-%v", first, second)
}



type Snapshot struct {
	Nodes []NodeSnapshot `json:"nodes,omitempty"`
	Conns []Conn         `json:"conns,omitempty"`
}


type NodeSnapshot struct {
	Node Node `json:"node,omitempty"`

	
	Snapshots map[string][]byte `json:"snapshots,omitempty"`
}


func (net *Network) Snapshot() (*Snapshot, error) {
	return net.snapshot(nil, nil)
}

func (net *Network) SnapshotWithServices(addServices []string, removeServices []string) (*Snapshot, error) {
	return net.snapshot(addServices, removeServices)
}

func (net *Network) snapshot(addServices []string, removeServices []string) (*Snapshot, error) {
	net.lock.Lock()
	defer net.lock.Unlock()
	snap := &Snapshot{
		Nodes: make([]NodeSnapshot, len(net.Nodes)),
	}
	for i, node := range net.Nodes {
		snap.Nodes[i] = NodeSnapshot{Node: *node.copy()}
		if !node.Up() {
			continue
		}
		snapshots, err := node.Snapshots()
		if err != nil {
			return nil, err
		}
		snap.Nodes[i].Snapshots = snapshots
		for _, addSvc := range addServices {
			haveSvc := false
			for _, svc := range snap.Nodes[i].Node.Config.Lifecycles {
				if svc == addSvc {
					haveSvc = true
					break
				}
			}
			if !haveSvc {
				snap.Nodes[i].Node.Config.Lifecycles = append(snap.Nodes[i].Node.Config.Lifecycles, addSvc)
			}
		}
		if len(removeServices) > 0 {
			var cleanedServices []string
			for _, svc := range snap.Nodes[i].Node.Config.Lifecycles {
				haveSvc := false
				for _, rmSvc := range removeServices {
					if rmSvc == svc {
						haveSvc = true
						break
					}
				}
				if !haveSvc {
					cleanedServices = append(cleanedServices, svc)
				}

			}
			snap.Nodes[i].Node.Config.Lifecycles = cleanedServices
		}
	}
	for _, conn := range net.Conns {
		if conn.Up {
			snap.Conns = append(snap.Conns, *conn)
		}
	}
	return snap, nil
}


var snapshotLoadTimeout = 900 * time.Second


func (net *Network) Load(snap *Snapshot) error {
	
	for _, n := range snap.Nodes {
		if _, err := net.NewNodeWithConfig(n.Node.Config); err != nil {
			return err
		}
		if !n.Node.Up() {
			continue
		}
		if err := net.startWithSnapshots(n.Node.Config.ID, n.Snapshots); err != nil {
			return err
		}
	}

	
	allConnected := make(chan struct{}) 
	done := make(chan struct{})         
	defer close(done)

	
	
	
	events := make(chan *Event)
	sub := net.Events().Subscribe(events)
	defer sub.Unsubscribe()

	go func() {
		
		total := len(snap.Conns)
		
		
		connections := make(map[[2]enode.ID]struct{}, total)

		for {
			select {
			case e := <-events:
				
				
				if e.Control {
					continue
				}
				
				if e.Type != EventTypeConn {
					continue
				}
				connection := [2]enode.ID{e.Conn.One, e.Conn.Other}
				
				if !e.Conn.Up {
					
					
					delete(connections, connection)
					log.Warn("load snapshot: unexpected disconnection", "one", e.Conn.One, "other", e.Conn.Other)
					continue
				}
				
				for _, conn := range snap.Conns {
					if conn.One == e.Conn.One && conn.Other == e.Conn.Other {
						
						connections[connection] = struct{}{}
						if len(connections) == total {
							
							close(allConnected)
							return
						}

						break
					}
				}
			case <-done:
				
				return
			}
		}
	}()

	
	for _, conn := range snap.Conns {

		if !net.GetNode(conn.One).Up() || !net.GetNode(conn.Other).Up() {
			
			
			continue
		}
		if err := net.Connect(conn.One, conn.Other); err != nil {
			return err
		}
	}

	select {
	
	case <-allConnected:
	
	case <-time.After(snapshotLoadTimeout):
		return errors.New("snapshot connections not established")
	}
	return nil
}


func (net *Network) Subscribe(events chan *Event) {
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if event.Control {
				net.executeControlEvent(event)
			}
		case <-net.quitc:
			return
		}
	}
}

func (net *Network) executeControlEvent(event *Event) {
	log.Trace("Executing control event", "type", event.Type, "event", event)
	switch event.Type {
	case EventTypeNode:
		if err := net.executeNodeEvent(event); err != nil {
			log.Error("Error executing node event", "event", event, "err", err)
		}
	case EventTypeConn:
		if err := net.executeConnEvent(event); err != nil {
			log.Error("Error executing conn event", "event", event, "err", err)
		}
	case EventTypeMsg:
		log.Warn("Ignoring control msg event")
	}
}

func (net *Network) executeNodeEvent(e *Event) error {
	if !e.Node.Up() {
		return net.Stop(e.Node.ID())
	}

	if _, err := net.NewNodeWithConfig(e.Node.Config); err != nil {
		return err
	}
	return net.Start(e.Node.ID())
}

func (net *Network) executeConnEvent(e *Event) error {
	if e.Conn.Up {
		return net.Connect(e.Conn.One, e.Conn.Other)
	} else {
		return net.Disconnect(e.Conn.One, e.Conn.Other)
	}
}
