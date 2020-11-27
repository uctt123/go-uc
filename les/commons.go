















package les

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/les/checkpointoracle"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
)

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

func lesTopic(genesisHash common.Hash, protocolVersion uint) discv5.Topic {
	var name string
	switch protocolVersion {
	case lpv2:
		name = "LES2"
	default:
		panic(nil)
	}
	return discv5.Topic(name + "@" + common.Bytes2Hex(genesisHash.Bytes()[0:8]))
}

type chainReader interface {
	CurrentHeader() *types.Header
}


type lesCommons struct {
	genesis                      common.Hash
	config                       *eth.Config
	chainConfig                  *params.ChainConfig
	iConfig                      *light.IndexerConfig
	chainDb                      ethdb.Database
	chainReader                  chainReader
	chtIndexer, bloomTrieIndexer *core.ChainIndexer
	oracle                       *checkpointoracle.CheckpointOracle

	closeCh chan struct{}
	wg      sync.WaitGroup
}



type NodeInfo struct {
	Network    uint64                   `json:"network"`    
	Difficulty *big.Int                 `json:"difficulty"` 
	Genesis    common.Hash              `json:"genesis"`    
	Config     *params.ChainConfig      `json:"config"`     
	Head       common.Hash              `json:"head"`       
	CHT        params.TrustedCheckpoint `json:"cht"`        
}


func (c *lesCommons) makeProtocols(versions []uint, runPeer func(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) error, peerInfo func(id enode.ID) interface{}, dialCandidates enode.Iterator) []p2p.Protocol {
	protos := make([]p2p.Protocol, len(versions))
	for i, version := range versions {
		version := version
		protos[i] = p2p.Protocol{
			Name:     "les",
			Version:  version,
			Length:   ProtocolLengths[version],
			NodeInfo: c.nodeInfo,
			Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
				return runPeer(version, peer, rw)
			},
			PeerInfo:       peerInfo,
			DialCandidates: dialCandidates,
		}
	}
	return protos
}


func (c *lesCommons) nodeInfo() interface{} {
	head := c.chainReader.CurrentHeader()
	hash := head.Hash()
	return &NodeInfo{
		Network:    c.config.NetworkId,
		Difficulty: rawdb.ReadTd(c.chainDb, hash, head.Number.Uint64()),
		Genesis:    c.genesis,
		Config:     c.chainConfig,
		Head:       hash,
		CHT:        c.latestLocalCheckpoint(),
	}
}




func (c *lesCommons) latestLocalCheckpoint() params.TrustedCheckpoint {
	sections, _, _ := c.chtIndexer.Sections()
	sections2, _, _ := c.bloomTrieIndexer.Sections()
	
	if sections > sections2 {
		sections = sections2
	}
	if sections == 0 {
		
		return params.TrustedCheckpoint{}
	}
	return c.localCheckpoint(sections - 1)
}






func (c *lesCommons) localCheckpoint(index uint64) params.TrustedCheckpoint {
	sectionHead := c.chtIndexer.SectionHead(index)
	return params.TrustedCheckpoint{
		SectionIndex: index,
		SectionHead:  sectionHead,
		CHTRoot:      light.GetChtRoot(c.chainDb, index, sectionHead),
		BloomRoot:    light.GetBloomTrieRoot(c.chainDb, index, sectionHead),
	}
}


func (c *lesCommons) setupOracle(node *node.Node, genesis common.Hash, ethconfig *eth.Config) *checkpointoracle.CheckpointOracle {
	config := ethconfig.CheckpointOracle
	if config == nil {
		
		config = params.CheckpointOracles[genesis]
	}
	if config == nil {
		log.Info("Checkpoint registrar is not enabled")
		return nil
	}
	if config.Address == (common.Address{}) || uint64(len(config.Signers)) < config.Threshold {
		log.Warn("Invalid checkpoint registrar config")
		return nil
	}
	oracle := checkpointoracle.New(config, c.localCheckpoint)
	rpcClient, _ := node.Attach()
	client := ethclient.NewClient(rpcClient)
	oracle.Start(client)
	log.Info("Configured checkpoint registrar", "address", config.Address, "signers", len(config.Signers), "threshold", config.Threshold)
	return oracle
}
