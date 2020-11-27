















package adapters

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/docker/docker/pkg/reexec"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/websocket"
)








type Node interface {
	
	Addr() []byte

	
	
	Client() (*rpc.Client, error)

	
	ServeRPC(*websocket.Conn) error

	
	Start(snapshots map[string][]byte) error

	
	Stop() error

	
	NodeInfo() *p2p.NodeInfo

	
	Snapshots() (map[string][]byte, error)
}


type NodeAdapter interface {
	
	Name() string

	
	NewNode(config *NodeConfig) (Node, error)
}



type NodeConfig struct {
	
	
	ID enode.ID

	
	
	PrivateKey *ecdsa.PrivateKey

	
	EnableMsgEvents bool

	
	Name string

	
	DataDir string

	
	
	
	
	Lifecycles []string

	
	
	
	Properties []string

	
	node *enode.Node

	
	Record enr.Record

	
	Reachable func(id enode.ID) bool

	Port uint16
}



type nodeConfigJSON struct {
	ID              string   `json:"id"`
	PrivateKey      string   `json:"private_key"`
	Name            string   `json:"name"`
	Services        []string `json:"services"`
	Properties      []string `json:"properties"`
	EnableMsgEvents bool     `json:"enable_msg_events"`
	Port            uint16   `json:"port"`
}



func (n *NodeConfig) MarshalJSON() ([]byte, error) {
	confJSON := nodeConfigJSON{
		ID:              n.ID.String(),
		Name:            n.Name,
		Services:        n.Lifecycles,
		Properties:      n.Properties,
		Port:            n.Port,
		EnableMsgEvents: n.EnableMsgEvents,
	}
	if n.PrivateKey != nil {
		confJSON.PrivateKey = hex.EncodeToString(crypto.FromECDSA(n.PrivateKey))
	}
	return json.Marshal(confJSON)
}



func (n *NodeConfig) UnmarshalJSON(data []byte) error {
	var confJSON nodeConfigJSON
	if err := json.Unmarshal(data, &confJSON); err != nil {
		return err
	}

	if confJSON.ID != "" {
		if err := n.ID.UnmarshalText([]byte(confJSON.ID)); err != nil {
			return err
		}
	}

	if confJSON.PrivateKey != "" {
		key, err := hex.DecodeString(confJSON.PrivateKey)
		if err != nil {
			return err
		}
		privKey, err := crypto.ToECDSA(key)
		if err != nil {
			return err
		}
		n.PrivateKey = privKey
	}

	n.Name = confJSON.Name
	n.Lifecycles = confJSON.Services
	n.Properties = confJSON.Properties
	n.Port = confJSON.Port
	n.EnableMsgEvents = confJSON.EnableMsgEvents

	return nil
}


func (n *NodeConfig) Node() *enode.Node {
	return n.node
}



func RandomNodeConfig() *NodeConfig {
	prvkey, err := crypto.GenerateKey()
	if err != nil {
		panic("unable to generate key")
	}

	port, err := assignTCPPort()
	if err != nil {
		panic("unable to assign tcp port")
	}

	enodId := enode.PubkeyToIDV4(&prvkey.PublicKey)
	return &NodeConfig{
		PrivateKey:      prvkey,
		ID:              enodId,
		Name:            fmt.Sprintf("node_%s", enodId.String()),
		Port:            port,
		EnableMsgEvents: true,
	}
}

func assignTCPPort() (uint16, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}
	p, err := strconv.ParseInt(port, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint16(p), nil
}



type ServiceContext struct {
	RPCDialer

	Config   *NodeConfig
	Snapshot []byte
}




type RPCDialer interface {
	DialRPC(id enode.ID) (*rpc.Client, error)
}




type LifecycleConstructor func(ctx *ServiceContext, stack *node.Node) (node.Lifecycle, error)


type LifecycleConstructors map[string]LifecycleConstructor



var lifecycleConstructorFuncs = make(LifecycleConstructors)






func RegisterLifecycles(lifecycles LifecycleConstructors) {
	for name, f := range lifecycles {
		if _, exists := lifecycleConstructorFuncs[name]; exists {
			panic(fmt.Sprintf("node service already exists: %q", name))
		}
		lifecycleConstructorFuncs[name] = f
	}

	
	
	
	if reexec.Init() {
		os.Exit(0)
	}
}



func (n *NodeConfig) initEnode(ip net.IP, tcpport int, udpport int) error {
	enrIp := enr.IP(ip)
	n.Record.Set(&enrIp)
	enrTcpPort := enr.TCP(tcpport)
	n.Record.Set(&enrTcpPort)
	enrUdpPort := enr.UDP(udpport)
	n.Record.Set(&enrUdpPort)

	err := enode.SignV4(&n.Record, n.PrivateKey)
	if err != nil {
		return fmt.Errorf("unable to generate ENR: %v", err)
	}
	nod, err := enode.New(enode.V4ID{}, &n.Record)
	if err != nil {
		return fmt.Errorf("unable to create enode: %v", err)
	}
	log.Trace("simnode new", "record", n.Record)
	n.node = nod
	return nil
}

func (n *NodeConfig) initDummyEnode() error {
	return n.initEnode(net.IPv4(127, 0, 0, 1), int(n.Port), 0)
}
