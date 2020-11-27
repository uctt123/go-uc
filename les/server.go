















package les

import (
	"crypto/ecdsa"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/les/flowcontrol"
	lps "github.com/ethereum/go-ethereum/les/lespay/server"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

type LesServer struct {
	lesCommons

	archiveMode bool 
	peers       *clientPeerSet
	serverset   *serverSet
	handler     *serverHandler
	lesTopics   []discv5.Topic
	privateKey  *ecdsa.PrivateKey

	
	fcManager    *flowcontrol.ClientManager
	costTracker  *costTracker
	defParams    flowcontrol.ServerParams
	servingQueue *servingQueue
	clientPool   *clientPool

	minCapacity, maxCapacity uint64
	threadsIdle              int 
	threadsBusy              int 

	p2pSrv *p2p.Server
}

func NewLesServer(node *node.Node, e *eth.Ethereum, config *eth.Config) (*LesServer, error) {
	
	lesTopics := make([]discv5.Topic, len(AdvertiseProtocolVersions))
	for i, pv := range AdvertiseProtocolVersions {
		lesTopics[i] = lesTopic(e.BlockChain().Genesis().Hash(), pv)
	}
	
	
	threads := config.LightServ * 4 / 100
	if threads < 4 {
		threads = 4
	}
	srv := &LesServer{
		lesCommons: lesCommons{
			genesis:          e.BlockChain().Genesis().Hash(),
			config:           config,
			chainConfig:      e.BlockChain().Config(),
			iConfig:          light.DefaultServerIndexerConfig,
			chainDb:          e.ChainDb(),
			chainReader:      e.BlockChain(),
			chtIndexer:       light.NewChtIndexer(e.ChainDb(), nil, params.CHTFrequency, params.HelperTrieProcessConfirmations, true),
			bloomTrieIndexer: light.NewBloomTrieIndexer(e.ChainDb(), nil, params.BloomBitsBlocks, params.BloomTrieFrequency, true),
			closeCh:          make(chan struct{}),
		},
		archiveMode:  e.ArchiveMode(),
		peers:        newClientPeerSet(),
		serverset:    newServerSet(),
		lesTopics:    lesTopics,
		fcManager:    flowcontrol.NewClientManager(nil, &mclock.System{}),
		servingQueue: newServingQueue(int64(time.Millisecond*10), float64(config.LightServ)/100),
		threadsBusy:  config.LightServ/100 + 1,
		threadsIdle:  threads,
		p2pSrv:       node.Server(),
	}
	srv.handler = newServerHandler(srv, e.BlockChain(), e.ChainDb(), e.TxPool(), e.Synced)
	srv.costTracker, srv.minCapacity = newCostTracker(e.ChainDb(), config)
	srv.oracle = srv.setupOracle(node, e.BlockChain().Genesis().Hash(), config)

	
	e.BloomIndexer().AddChildIndexer(srv.bloomTrieIndexer)

	
	srv.defParams = flowcontrol.ServerParams{
		BufLimit:    srv.minCapacity * bufLimitRatio,
		MinRecharge: srv.minCapacity,
	}
	
	
	
	
	
	totalRecharge := srv.costTracker.totalRecharge()
	srv.maxCapacity = srv.minCapacity * uint64(srv.config.LightPeers)
	if totalRecharge > srv.maxCapacity {
		srv.maxCapacity = totalRecharge
	}
	srv.fcManager.SetCapacityLimits(srv.minCapacity, srv.maxCapacity, srv.minCapacity*2)
	srv.clientPool = newClientPool(srv.chainDb, srv.minCapacity, defaultConnectedBias, mclock.System{}, func(id enode.ID) { go srv.peers.unregister(id.String()) })
	srv.clientPool.setDefaultFactors(lps.PriceFactors{TimeFactor: 0, CapacityFactor: 1, RequestFactor: 1}, lps.PriceFactors{TimeFactor: 0, CapacityFactor: 1, RequestFactor: 1})

	checkpoint := srv.latestLocalCheckpoint()
	if !checkpoint.Empty() {
		log.Info("Loaded latest checkpoint", "section", checkpoint.SectionIndex, "head", checkpoint.SectionHead,
			"chtroot", checkpoint.CHTRoot, "bloomroot", checkpoint.BloomRoot)
	}
	srv.chtIndexer.Start(e.BlockChain())

	node.RegisterProtocols(srv.Protocols())
	node.RegisterAPIs(srv.APIs())
	node.RegisterLifecycle(srv)

	return srv, nil
}

func (s *LesServer) APIs() []rpc.API {
	return []rpc.API{
		{
			Namespace: "les",
			Version:   "1.0",
			Service:   NewPrivateLightAPI(&s.lesCommons),
			Public:    false,
		},
		{
			Namespace: "les",
			Version:   "1.0",
			Service:   NewPrivateLightServerAPI(s),
			Public:    false,
		},
		{
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s),
			Public:    false,
		},
	}
}

func (s *LesServer) Protocols() []p2p.Protocol {
	ps := s.makeProtocols(ServerProtocolVersions, s.handler.runPeer, func(id enode.ID) interface{} {
		if p := s.peers.peer(id.String()); p != nil {
			return p.Info()
		}
		return nil
	}, nil)
	
	for i := range ps {
		ps[i].Attributes = []enr.Entry{&lesEntry{}}
	}
	return ps
}


func (s *LesServer) Start() error {
	s.privateKey = s.p2pSrv.PrivateKey
	s.handler.start()

	s.wg.Add(1)
	go s.capacityManagement()

	if s.p2pSrv.DiscV5 != nil {
		for _, topic := range s.lesTopics {
			topic := topic
			go func() {
				logger := log.New("topic", topic)
				logger.Info("Starting topic registration")
				defer logger.Info("Terminated topic registration")

				s.p2pSrv.DiscV5.RegisterTopic(topic, s.closeCh)
			}()
		}
	}

	return nil
}


func (s *LesServer) Stop() error {
	close(s.closeCh)

	
	s.serverset.close()

	
	
	
	
	s.peers.close()

	s.fcManager.Stop()
	s.costTracker.stop()
	s.handler.stop()
	s.clientPool.stop() 
	s.servingQueue.stop()

	
	s.chtIndexer.Close()
	s.wg.Wait()
	log.Info("Les server stopped")

	return nil
}




func (s *LesServer) capacityManagement() {
	defer s.wg.Done()

	processCh := make(chan bool, 100)
	sub := s.handler.blockchain.SubscribeBlockProcessingEvent(processCh)
	defer sub.Unsubscribe()

	totalRechargeCh := make(chan uint64, 100)
	totalRecharge := s.costTracker.subscribeTotalRecharge(totalRechargeCh)

	totalCapacityCh := make(chan uint64, 100)
	totalCapacity := s.fcManager.SubscribeTotalCapacity(totalCapacityCh)
	s.clientPool.setLimits(s.config.LightPeers, totalCapacity)

	var (
		busy         bool
		freePeers    uint64
		blockProcess mclock.AbsTime
	)
	updateRecharge := func() {
		if busy {
			s.servingQueue.setThreads(s.threadsBusy)
			s.fcManager.SetRechargeCurve(flowcontrol.PieceWiseLinear{{0, 0}, {totalRecharge, totalRecharge}})
		} else {
			s.servingQueue.setThreads(s.threadsIdle)
			s.fcManager.SetRechargeCurve(flowcontrol.PieceWiseLinear{{0, 0}, {totalRecharge / 10, totalRecharge}, {totalRecharge, totalRecharge}})
		}
	}
	updateRecharge()

	for {
		select {
		case busy = <-processCh:
			if busy {
				blockProcess = mclock.Now()
			} else {
				blockProcessingTimer.Update(time.Duration(mclock.Now() - blockProcess))
			}
			updateRecharge()
		case totalRecharge = <-totalRechargeCh:
			totalRechargeGauge.Update(int64(totalRecharge))
			updateRecharge()
		case totalCapacity = <-totalCapacityCh:
			totalCapacityGauge.Update(int64(totalCapacity))
			newFreePeers := totalCapacity / s.minCapacity
			if newFreePeers < freePeers && newFreePeers < uint64(s.config.LightPeers) {
				log.Warn("Reduced free peer connections", "from", freePeers, "to", newFreePeers)
			}
			freePeers = newFreePeers
			s.clientPool.setLimits(s.config.LightPeers, totalCapacity)
		case <-s.closeCh:
			return
		}
	}
}
