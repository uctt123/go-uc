


















package geth

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethstats"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/les"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/params"
)





type NodeConfig struct {
	
	BootstrapNodes *Enodes

	
	
	MaxPeers int

	
	EthereumEnabled bool

	
	
	EthereumNetworkID int64 

	
	
	EthereumGenesis string

	
	
	EthereumDatabaseCache int

	
	
	
	
	EthereumNetStats string

	
	PprofAddress string
}



var defaultNodeConfig = &NodeConfig{
	BootstrapNodes:        FoundationBootnodes(),
	MaxPeers:              25,
	EthereumEnabled:       true,
	EthereumNetworkID:     1,
	EthereumDatabaseCache: 16,
}


func NewNodeConfig() *NodeConfig {
	config := *defaultNodeConfig
	return &config
}


func (conf *NodeConfig) AddBootstrapNode(node *Enode) {
	conf.BootstrapNodes.Append(node)
}


func (conf *NodeConfig) EncodeJSON() (string, error) {
	data, err := json.Marshal(conf)
	return string(data), err
}


func (conf *NodeConfig) String() string {
	return encodeOrError(conf)
}


type Node struct {
	node *node.Node
}


func NewNode(datadir string, config *NodeConfig) (stack *Node, _ error) {
	
	if config == nil {
		config = NewNodeConfig()
	}
	if config.MaxPeers == 0 {
		config.MaxPeers = defaultNodeConfig.MaxPeers
	}
	if config.BootstrapNodes == nil || config.BootstrapNodes.Size() == 0 {
		config.BootstrapNodes = defaultNodeConfig.BootstrapNodes
	}

	if config.PprofAddress != "" {
		debug.StartPProf(config.PprofAddress, true)
	}

	
	nodeConf := &node.Config{
		Name:        clientIdentifier,
		Version:     params.VersionWithMeta,
		DataDir:     datadir,
		KeyStoreDir: filepath.Join(datadir, "keystore"), 
		P2P: p2p.Config{
			NoDiscovery:      true,
			DiscoveryV5:      true,
			BootstrapNodesV5: config.BootstrapNodes.nodes,
			ListenAddr:       ":0",
			NAT:              nat.Any(),
			MaxPeers:         config.MaxPeers,
		},
	}

	rawStack, err := node.New(nodeConf)
	if err != nil {
		return nil, err
	}

	debug.Memsize.Add("node", rawStack)

	var genesis *core.Genesis
	if config.EthereumGenesis != "" {
		
		genesis = new(core.Genesis)
		if err := json.Unmarshal([]byte(config.EthereumGenesis), genesis); err != nil {
			return nil, fmt.Errorf("invalid genesis spec: %v", err)
		}
		
		if config.EthereumGenesis == RopstenGenesis() {
			genesis.Config = params.RopstenChainConfig
			if config.EthereumNetworkID == 1 {
				config.EthereumNetworkID = 3
			}
		}
		
		if config.EthereumGenesis == RinkebyGenesis() {
			genesis.Config = params.RinkebyChainConfig
			if config.EthereumNetworkID == 1 {
				config.EthereumNetworkID = 4
			}
		}
		
		if config.EthereumGenesis == GoerliGenesis() {
			genesis.Config = params.GoerliChainConfig
			if config.EthereumNetworkID == 1 {
				config.EthereumNetworkID = 5
			}
		}
	}
	
	if config.EthereumEnabled {
		ethConf := eth.DefaultConfig
		ethConf.Genesis = genesis
		ethConf.SyncMode = downloader.LightSync
		ethConf.NetworkId = uint64(config.EthereumNetworkID)
		ethConf.DatabaseCache = config.EthereumDatabaseCache
		lesBackend, err := les.New(rawStack, &ethConf)
		if err != nil {
			return nil, fmt.Errorf("ethereum init: %v", err)
		}
		
		if config.EthereumNetStats != "" {
			if err := ethstats.New(rawStack, lesBackend.ApiBackend, lesBackend.Engine(), config.EthereumNetStats); err != nil {
				return nil, fmt.Errorf("netstats init: %v", err)
			}
		}
	}
	return &Node{rawStack}, nil
}



func (n *Node) Close() error {
	return n.node.Close()
}


func (n *Node) Start() error {
	
	return n.node.Start()
}





func (n *Node) Stop() error {
	return n.node.Close()
}


func (n *Node) GetEthereumClient() (client *EthereumClient, _ error) {
	rpc, err := n.node.Attach()
	if err != nil {
		return nil, err
	}
	return &EthereumClient{ethclient.NewClient(rpc)}, nil
}


func (n *Node) GetNodeInfo() *NodeInfo {
	return &NodeInfo{n.node.Server().NodeInfo()}
}


func (n *Node) GetPeersInfo() *PeerInfos {
	return &PeerInfos{n.node.Server().PeersInfo()}
}
