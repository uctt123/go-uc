















package adapters

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/simulations/pipes"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/websocket"
)



type SimAdapter struct {
	pipe       func() (net.Conn, net.Conn, error)
	mtx        sync.RWMutex
	nodes      map[enode.ID]*SimNode
	lifecycles LifecycleConstructors
}





func NewSimAdapter(services LifecycleConstructors) *SimAdapter {
	return &SimAdapter{
		pipe:       pipes.NetPipe,
		nodes:      make(map[enode.ID]*SimNode),
		lifecycles: services,
	}
}


func (s *SimAdapter) Name() string {
	return "sim-adapter"
}


func (s *SimAdapter) NewNode(config *NodeConfig) (Node, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	id := config.ID
	
	if config.PrivateKey == nil {
		return nil, fmt.Errorf("node is missing private key: %s", id)
	}

	
	if _, exists := s.nodes[id]; exists {
		return nil, fmt.Errorf("node already exists: %s", id)
	}

	
	if len(config.Lifecycles) == 0 {
		return nil, errors.New("node must have at least one service")
	}
	for _, service := range config.Lifecycles {
		if _, exists := s.lifecycles[service]; !exists {
			return nil, fmt.Errorf("unknown node service %q", service)
		}
	}

	err := config.initDummyEnode()
	if err != nil {
		return nil, err
	}

	n, err := node.New(&node.Config{
		P2P: p2p.Config{
			PrivateKey:      config.PrivateKey,
			MaxPeers:        math.MaxInt32,
			NoDiscovery:     true,
			Dialer:          s,
			EnableMsgEvents: config.EnableMsgEvents,
		},
		NoUSB:  true,
		Logger: log.New("node.id", id.String()),
	})
	if err != nil {
		return nil, err
	}

	simNode := &SimNode{
		ID:      id,
		config:  config,
		node:    n,
		adapter: s,
		running: make(map[string]node.Lifecycle),
	}
	s.nodes[id] = simNode
	return simNode, nil
}



func (s *SimAdapter) Dial(ctx context.Context, dest *enode.Node) (conn net.Conn, err error) {
	node, ok := s.GetNode(dest.ID())
	if !ok {
		return nil, fmt.Errorf("unknown node: %s", dest.ID())
	}
	srv := node.Server()
	if srv == nil {
		return nil, fmt.Errorf("node not running: %s", dest.ID())
	}
	
	pipe1, pipe2, err := s.pipe()
	if err != nil {
		return nil, err
	}
	
	
	
	go srv.SetupConn(pipe1, 0, nil)
	return pipe2, nil
}



func (s *SimAdapter) DialRPC(id enode.ID) (*rpc.Client, error) {
	node, ok := s.GetNode(id)
	if !ok {
		return nil, fmt.Errorf("unknown node: %s", id)
	}
	return node.node.Attach()
}


func (s *SimAdapter) GetNode(id enode.ID) (*SimNode, bool) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	node, ok := s.nodes[id]
	return node, ok
}




type SimNode struct {
	lock         sync.RWMutex
	ID           enode.ID
	config       *NodeConfig
	adapter      *SimAdapter
	node         *node.Node
	running      map[string]node.Lifecycle
	client       *rpc.Client
	registerOnce sync.Once
}



func (sn *SimNode) Close() error {
	return sn.node.Close()
}


func (sn *SimNode) Addr() []byte {
	return []byte(sn.Node().String())
}


func (sn *SimNode) Node() *enode.Node {
	return sn.config.Node()
}



func (sn *SimNode) Client() (*rpc.Client, error) {
	sn.lock.RLock()
	defer sn.lock.RUnlock()
	if sn.client == nil {
		return nil, errors.New("node not started")
	}
	return sn.client, nil
}



func (sn *SimNode) ServeRPC(conn *websocket.Conn) error {
	handler, err := sn.node.RPCHandler()
	if err != nil {
		return err
	}
	codec := rpc.NewFuncCodec(conn, conn.WriteJSON, conn.ReadJSON)
	handler.ServeCodec(codec, 0)
	return nil
}



func (sn *SimNode) Snapshots() (map[string][]byte, error) {
	sn.lock.RLock()
	services := make(map[string]node.Lifecycle, len(sn.running))
	for name, service := range sn.running {
		services[name] = service
	}
	sn.lock.RUnlock()
	if len(services) == 0 {
		return nil, errors.New("no running services")
	}
	snapshots := make(map[string][]byte)
	for name, service := range services {
		if s, ok := service.(interface {
			Snapshot() ([]byte, error)
		}); ok {
			snap, err := s.Snapshot()
			if err != nil {
				return nil, err
			}
			snapshots[name] = snap
		}
	}
	return snapshots, nil
}


func (sn *SimNode) Start(snapshots map[string][]byte) error {
	
	
	var regErr error
	sn.registerOnce.Do(func() {
		for _, name := range sn.config.Lifecycles {
			ctx := &ServiceContext{
				RPCDialer: sn.adapter,
				Config:    sn.config,
			}
			if snapshots != nil {
				ctx.Snapshot = snapshots[name]
			}
			serviceFunc := sn.adapter.lifecycles[name]
			service, err := serviceFunc(ctx, sn.node)
			if err != nil {
				regErr = err
				break
			}
			
			if _, ok := sn.running[name]; ok {
				continue
			}
			sn.running[name] = service
			sn.node.RegisterLifecycle(service)
		}
	})
	if regErr != nil {
		return regErr
	}

	if err := sn.node.Start(); err != nil {
		return err
	}

	
	client, err := sn.node.Attach()
	if err != nil {
		return err
	}
	sn.lock.Lock()
	sn.client = client
	sn.lock.Unlock()

	return nil
}


func (sn *SimNode) Stop() error {
	sn.lock.Lock()
	if sn.client != nil {
		sn.client.Close()
		sn.client = nil
	}
	sn.lock.Unlock()
	return sn.node.Close()
}


func (sn *SimNode) Service(name string) node.Lifecycle {
	sn.lock.RLock()
	defer sn.lock.RUnlock()
	return sn.running[name]
}


func (sn *SimNode) Services() []node.Lifecycle {
	sn.lock.RLock()
	defer sn.lock.RUnlock()
	services := make([]node.Lifecycle, 0, len(sn.running))
	for _, service := range sn.running {
		services = append(services, service)
	}
	return services
}


func (sn *SimNode) ServiceMap() map[string]node.Lifecycle {
	sn.lock.RLock()
	defer sn.lock.RUnlock()
	services := make(map[string]node.Lifecycle, len(sn.running))
	for name, service := range sn.running {
		services[name] = service
	}
	return services
}


func (sn *SimNode) Server() *p2p.Server {
	return sn.node.Server()
}



func (sn *SimNode) SubscribeEvents(ch chan *p2p.PeerEvent) event.Subscription {
	srv := sn.Server()
	if srv == nil {
		panic("node not running")
	}
	return srv.SubscribeEvents(ch)
}


func (sn *SimNode) NodeInfo() *p2p.NodeInfo {
	server := sn.Server()
	if server == nil {
		return &p2p.NodeInfo{
			ID:    sn.ID.String(),
			Enode: sn.Node().String(),
		}
	}
	return server.NodeInfo()
}
