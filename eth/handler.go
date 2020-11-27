















package eth

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/fetcher"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

const (
	softResponseLimit = 2 * 1024 * 1024 
	estHeaderRlpSize  = 500             

	
	
	txChanSize = 4096
)

var (
	syncChallengeTimeout = 15 * time.Second 
)

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

type ProtocolManager struct {
	networkID  uint64
	forkFilter forkid.Filter 

	fastSync  uint32 
	acceptTxs uint32 

	checkpointNumber uint64      
	checkpointHash   common.Hash 

	txpool     txPool
	blockchain *core.BlockChain
	chaindb    ethdb.Database
	maxPeers   int

	downloader   *downloader.Downloader
	blockFetcher *fetcher.BlockFetcher
	txFetcher    *fetcher.TxFetcher
	peers        *peerSet

	eventMux      *event.TypeMux
	txsCh         chan core.NewTxsEvent
	txsSub        event.Subscription
	minedBlockSub *event.TypeMuxSubscription

	whitelist map[uint64]common.Hash

	
	txsyncCh chan *txsync
	quitSync chan struct{}

	chainSync *chainSyncer
	wg        sync.WaitGroup
	peerWG    sync.WaitGroup

	
	broadcastTxAnnouncesOnly bool 
}



func NewProtocolManager(config *params.ChainConfig, checkpoint *params.TrustedCheckpoint, mode downloader.SyncMode, networkID uint64, mux *event.TypeMux, txpool txPool, engine consensus.Engine, blockchain *core.BlockChain, chaindb ethdb.Database, cacheLimit int, whitelist map[uint64]common.Hash) (*ProtocolManager, error) {
	
	manager := &ProtocolManager{
		networkID:  networkID,
		forkFilter: forkid.NewFilter(blockchain),
		eventMux:   mux,
		txpool:     txpool,
		blockchain: blockchain,
		chaindb:    chaindb,
		peers:      newPeerSet(),
		whitelist:  whitelist,
		txsyncCh:   make(chan *txsync),
		quitSync:   make(chan struct{}),
	}

	if mode == downloader.FullSync {
		
		
		
		
		
		
		
		
		fullBlock, fastBlock := blockchain.CurrentBlock(), blockchain.CurrentFastBlock()
		if fullBlock.NumberU64() == 0 && fastBlock.NumberU64() > 0 {
			manager.fastSync = uint32(1)
			log.Warn("Switch sync mode from full sync to fast sync")
		}
	} else {
		if blockchain.CurrentBlock().NumberU64() > 0 {
			
			log.Warn("Switch sync mode from fast sync to full sync")
		} else {
			
			manager.fastSync = uint32(1)
		}
	}

	
	if checkpoint != nil {
		manager.checkpointNumber = (checkpoint.SectionIndex+1)*params.CHTFrequency - 1
		manager.checkpointHash = checkpoint.SectionHead
	}

	
	
	
	var stateBloom *trie.SyncBloom
	if atomic.LoadUint32(&manager.fastSync) == 1 {
		stateBloom = trie.NewSyncBloom(uint64(cacheLimit), chaindb)
	}
	manager.downloader = downloader.New(manager.checkpointNumber, chaindb, stateBloom, manager.eventMux, blockchain, nil, manager.removePeer)

	
	validator := func(header *types.Header) error {
		return engine.VerifyHeader(blockchain, header, true)
	}
	heighter := func() uint64 {
		return blockchain.CurrentBlock().NumberU64()
	}
	inserter := func(blocks types.Blocks) (int, error) {
		
		
		
		
		
		
		if manager.blockchain.CurrentBlock().NumberU64() < manager.checkpointNumber {
			log.Warn("Unsynced yet, discarded propagated block", "number", blocks[0].Number(), "hash", blocks[0].Hash())
			return 0, nil
		}
		
		
		
		
		
		if atomic.LoadUint32(&manager.fastSync) == 1 {
			log.Warn("Fast syncing, discarded propagated block", "number", blocks[0].Number(), "hash", blocks[0].Hash())
			return 0, nil
		}
		n, err := manager.blockchain.InsertChain(blocks)
		if err == nil {
			atomic.StoreUint32(&manager.acceptTxs, 1) 
		}
		return n, err
	}
	manager.blockFetcher = fetcher.NewBlockFetcher(false, nil, blockchain.GetBlockByHash, validator, manager.BroadcastBlock, heighter, nil, inserter, manager.removePeer)

	fetchTx := func(peer string, hashes []common.Hash) error {
		p := manager.peers.Peer(peer)
		if p == nil {
			return errors.New("unknown peer")
		}
		return p.RequestTxs(hashes)
	}
	manager.txFetcher = fetcher.NewTxFetcher(txpool.Has, txpool.AddRemotes, fetchTx)

	manager.chainSync = newChainSyncer(manager)

	return manager, nil
}

func (pm *ProtocolManager) makeProtocol(version uint) p2p.Protocol {
	length, ok := protocolLengths[version]
	if !ok {
		panic("makeProtocol for unknown version")
	}

	return p2p.Protocol{
		Name:    protocolName,
		Version: version,
		Length:  length,
		Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
			return pm.runPeer(pm.newPeer(int(version), p, rw, pm.txpool.Get))
		},
		NodeInfo: func() interface{} {
			return pm.NodeInfo()
		},
		PeerInfo: func(id enode.ID) interface{} {
			if p := pm.peers.Peer(fmt.Sprintf("%x", id[:8])); p != nil {
				return p.Info()
			}
			return nil
		},
	}
}

func (pm *ProtocolManager) removePeer(id string) {
	
	peer := pm.peers.Peer(id)
	if peer == nil {
		return
	}
	log.Debug("Removing Ethereum peer", "peer", id)

	
	pm.downloader.UnregisterPeer(id)
	pm.txFetcher.Drop(id)

	if err := pm.peers.Unregister(id); err != nil {
		log.Error("Peer removal failed", "peer", id, "err", err)
	}
	
	if peer != nil {
		peer.Peer.Disconnect(p2p.DiscUselessPeer)
	}
}

func (pm *ProtocolManager) Start(maxPeers int) {
	pm.maxPeers = maxPeers

	
	pm.wg.Add(1)
	pm.txsCh = make(chan core.NewTxsEvent, txChanSize)
	pm.txsSub = pm.txpool.SubscribeNewTxsEvent(pm.txsCh)
	go pm.txBroadcastLoop()

	
	pm.wg.Add(1)
	pm.minedBlockSub = pm.eventMux.Subscribe(core.NewMinedBlockEvent{})
	go pm.minedBroadcastLoop()

	
	pm.wg.Add(2)
	go pm.chainSync.loop()
	go pm.txsyncLoop64() 
}

func (pm *ProtocolManager) Stop() {
	pm.txsSub.Unsubscribe()        
	pm.minedBlockSub.Unsubscribe() 

	
	
	close(pm.quitSync)
	pm.wg.Wait()

	
	
	
	
	pm.peers.Close()
	pm.peerWG.Wait()

	log.Info("Ethereum protocol stopped")
}

func (pm *ProtocolManager) newPeer(pv int, p *p2p.Peer, rw p2p.MsgReadWriter, getPooledTx func(hash common.Hash) *types.Transaction) *peer {
	return newPeer(pv, p, rw, getPooledTx)
}

func (pm *ProtocolManager) runPeer(p *peer) error {
	if !pm.chainSync.handlePeerEvent(p) {
		return p2p.DiscQuitting
	}
	pm.peerWG.Add(1)
	defer pm.peerWG.Done()
	return pm.handle(p)
}



func (pm *ProtocolManager) handle(p *peer) error {
	
	if pm.peers.Len() >= pm.maxPeers && !p.Peer.Info().Network.Trusted {
		return p2p.DiscTooManyPeers
	}
	p.Log().Debug("Ethereum peer connected", "name", p.Name())

	
	var (
		genesis = pm.blockchain.Genesis()
		head    = pm.blockchain.CurrentHeader()
		hash    = head.Hash()
		number  = head.Number.Uint64()
		td      = pm.blockchain.GetTd(hash, number)
	)
	forkID := forkid.NewID(pm.blockchain.Config(), pm.blockchain.Genesis().Hash(), pm.blockchain.CurrentHeader().Number.Uint64())
	if err := p.Handshake(pm.networkID, td, hash, genesis.Hash(), forkID, pm.forkFilter); err != nil {
		p.Log().Debug("Ethereum handshake failed", "err", err)
		return err
	}

	
	if err := pm.peers.Register(p, pm.removePeer); err != nil {
		p.Log().Error("Ethereum peer registration failed", "err", err)
		return err
	}
	defer pm.removePeer(p.id)

	
	if err := pm.downloader.RegisterPeer(p.id, p.version, p); err != nil {
		return err
	}
	pm.chainSync.handlePeerEvent(p)

	
	
	pm.syncTransactions(p)

	
	if pm.checkpointHash != (common.Hash{}) {
		
		if err := p.RequestHeadersByNumber(pm.checkpointNumber, 1, 0, false); err != nil {
			return err
		}
		
		p.syncDrop = time.AfterFunc(syncChallengeTimeout, func() {
			p.Log().Warn("Checkpoint challenge timed out, dropping", "addr", p.RemoteAddr(), "type", p.Name())
			pm.removePeer(p.id)
		})
		
		defer func() {
			if p.syncDrop != nil {
				p.syncDrop.Stop()
				p.syncDrop = nil
			}
		}()
	}
	
	for number := range pm.whitelist {
		if err := p.RequestHeadersByNumber(number, 1, 0, false); err != nil {
			return err
		}
	}
	
	for {
		if err := pm.handleMsg(p); err != nil {
			p.Log().Debug("Ethereum message handling failed", "err", err)
			return err
		}
	}
}



func (pm *ProtocolManager) handleMsg(p *peer) error {
	
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > protocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, protocolMaxMsgSize)
	}
	defer msg.Discard()

	
	switch {
	case msg.Code == StatusMsg:
		
		return errResp(ErrExtraStatusMsg, "uncontrolled status message")

	
	case msg.Code == GetBlockHeadersMsg:
		
		var query getBlockHeadersData
		if err := msg.Decode(&query); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		hashMode := query.Origin.Hash != (common.Hash{})
		first := true
		maxNonCanonical := uint64(100)

		
		var (
			bytes   common.StorageSize
			headers []*types.Header
			unknown bool
		)
		for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < downloader.MaxHeaderFetch {
			
			var origin *types.Header
			if hashMode {
				if first {
					first = false
					origin = pm.blockchain.GetHeaderByHash(query.Origin.Hash)
					if origin != nil {
						query.Origin.Number = origin.Number.Uint64()
					}
				} else {
					origin = pm.blockchain.GetHeader(query.Origin.Hash, query.Origin.Number)
				}
			} else {
				origin = pm.blockchain.GetHeaderByNumber(query.Origin.Number)
			}
			if origin == nil {
				break
			}
			headers = append(headers, origin)
			bytes += estHeaderRlpSize

			
			switch {
			case hashMode && query.Reverse:
				
				ancestor := query.Skip + 1
				if ancestor == 0 {
					unknown = true
				} else {
					query.Origin.Hash, query.Origin.Number = pm.blockchain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
					unknown = (query.Origin.Hash == common.Hash{})
				}
			case hashMode && !query.Reverse:
				
				var (
					current = origin.Number.Uint64()
					next    = current + query.Skip + 1
				)
				if next <= current {
					infos, _ := json.MarshalIndent(p.Peer.Info(), "", "  ")
					p.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
					unknown = true
				} else {
					if header := pm.blockchain.GetHeaderByNumber(next); header != nil {
						nextHash := header.Hash()
						expOldHash, _ := pm.blockchain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
						if expOldHash == query.Origin.Hash {
							query.Origin.Hash, query.Origin.Number = nextHash, next
						} else {
							unknown = true
						}
					} else {
						unknown = true
					}
				}
			case query.Reverse:
				
				if query.Origin.Number >= query.Skip+1 {
					query.Origin.Number -= query.Skip + 1
				} else {
					unknown = true
				}

			case !query.Reverse:
				
				query.Origin.Number += query.Skip + 1
			}
		}
		return p.SendBlockHeaders(headers)

	case msg.Code == BlockHeadersMsg:
		
		var headers []*types.Header
		if err := msg.Decode(&headers); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		
		if len(headers) == 0 && p.syncDrop != nil {
			
			p.syncDrop.Stop()
			p.syncDrop = nil

			
			
			
			if atomic.LoadUint32(&pm.fastSync) == 1 {
				p.Log().Warn("Dropping unsynced node during fast sync", "addr", p.RemoteAddr(), "type", p.Name())
				return errors.New("unsynced node cannot serve fast sync")
			}
		}
		
		filter := len(headers) == 1
		if filter {
			
			if p.syncDrop != nil && headers[0].Number.Uint64() == pm.checkpointNumber {
				
				p.syncDrop.Stop()
				p.syncDrop = nil

				
				if headers[0].Hash() != pm.checkpointHash {
					return errors.New("checkpoint hash mismatch")
				}
				return nil
			}
			
			if want, ok := pm.whitelist[headers[0].Number.Uint64()]; ok {
				if hash := headers[0].Hash(); want != hash {
					p.Log().Info("Whitelist mismatch, dropping peer", "number", headers[0].Number.Uint64(), "hash", hash, "want", want)
					return errors.New("whitelist block mismatch")
				}
				p.Log().Debug("Whitelist block verified", "number", headers[0].Number.Uint64(), "hash", want)
			}
			
			headers = pm.blockFetcher.FilterHeaders(p.id, headers, time.Now())
		}
		if len(headers) > 0 || !filter {
			err := pm.downloader.DeliverHeaders(p.id, headers)
			if err != nil {
				log.Debug("Failed to deliver headers", "err", err)
			}
		}

	case msg.Code == GetBlockBodiesMsg:
		
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		
		var (
			hash   common.Hash
			bytes  int
			bodies []rlp.RawValue
		)
		for bytes < softResponseLimit && len(bodies) < downloader.MaxBlockFetch {
			
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			
			if data := pm.blockchain.GetBodyRLP(hash); len(data) != 0 {
				bodies = append(bodies, data)
				bytes += len(data)
			}
		}
		return p.SendBlockBodiesRLP(bodies)

	case msg.Code == BlockBodiesMsg:
		
		var request blockBodiesData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		
		transactions := make([][]*types.Transaction, len(request))
		uncles := make([][]*types.Header, len(request))

		for i, body := range request {
			transactions[i] = body.Transactions
			uncles[i] = body.Uncles
		}
		
		filter := len(transactions) > 0 || len(uncles) > 0
		if filter {
			transactions, uncles = pm.blockFetcher.FilterBodies(p.id, transactions, uncles, time.Now())
		}
		if len(transactions) > 0 || len(uncles) > 0 || !filter {
			err := pm.downloader.DeliverBodies(p.id, transactions, uncles)
			if err != nil {
				log.Debug("Failed to deliver bodies", "err", err)
			}
		}

	case p.version >= eth63 && msg.Code == GetNodeDataMsg:
		
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		
		var (
			hash  common.Hash
			bytes int
			data  [][]byte
		)
		for bytes < softResponseLimit && len(data) < downloader.MaxStateFetch {
			
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			
			
			
			if !pm.downloader.SyncBloomContains(hash[:]) {
				
				continue
			}
			entry, err := pm.blockchain.TrieNode(hash)
			if len(entry) == 0 || err != nil {
				
				entry, err = pm.blockchain.ContractCodeWithPrefix(hash)
			}
			if err == nil && len(entry) > 0 {
				data = append(data, entry)
				bytes += len(entry)
			}
		}
		return p.SendNodeData(data)

	case p.version >= eth63 && msg.Code == NodeDataMsg:
		
		var data [][]byte
		if err := msg.Decode(&data); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		
		if err := pm.downloader.DeliverNodeData(p.id, data); err != nil {
			log.Debug("Failed to deliver node state data", "err", err)
		}

	case p.version >= eth63 && msg.Code == GetReceiptsMsg:
		
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		
		var (
			hash     common.Hash
			bytes    int
			receipts []rlp.RawValue
		)
		for bytes < softResponseLimit && len(receipts) < downloader.MaxReceiptFetch {
			
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			
			results := pm.blockchain.GetReceiptsByHash(hash)
			if results == nil {
				if header := pm.blockchain.GetHeaderByHash(hash); header == nil || header.ReceiptHash != types.EmptyRootHash {
					continue
				}
			}
			
			if encoded, err := rlp.EncodeToBytes(results); err != nil {
				log.Error("Failed to encode receipt", "err", err)
			} else {
				receipts = append(receipts, encoded)
				bytes += len(encoded)
			}
		}
		return p.SendReceiptsRLP(receipts)

	case p.version >= eth63 && msg.Code == ReceiptsMsg:
		
		var receipts [][]*types.Receipt
		if err := msg.Decode(&receipts); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		
		if err := pm.downloader.DeliverReceipts(p.id, receipts); err != nil {
			log.Debug("Failed to deliver receipts", "err", err)
		}

	case msg.Code == NewBlockHashesMsg:
		var announces newBlockHashesData
		if err := msg.Decode(&announces); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		
		for _, block := range announces {
			p.MarkBlock(block.Hash)
		}
		
		unknown := make(newBlockHashesData, 0, len(announces))
		for _, block := range announces {
			if !pm.blockchain.HasBlock(block.Hash, block.Number) {
				unknown = append(unknown, block)
			}
		}
		for _, block := range unknown {
			pm.blockFetcher.Notify(p.id, block.Hash, block.Number, time.Now(), p.RequestOneHeader, p.RequestBodies)
		}

	case msg.Code == NewBlockMsg:
		
		var request newBlockData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		if hash := types.CalcUncleHash(request.Block.Uncles()); hash != request.Block.UncleHash() {
			log.Warn("Propagated block has invalid uncles", "have", hash, "exp", request.Block.UncleHash())
			break 
		}
		if hash := types.DeriveSha(request.Block.Transactions(), trie.NewStackTrie(nil)); hash != request.Block.TxHash() {
			log.Warn("Propagated block has invalid body", "have", hash, "exp", request.Block.TxHash())
			break 
		}
		if err := request.sanityCheck(); err != nil {
			return err
		}
		request.Block.ReceivedAt = msg.ReceivedAt
		request.Block.ReceivedFrom = p

		
		p.MarkBlock(request.Block.Hash())
		pm.blockFetcher.Enqueue(p.id, request.Block)

		
		
		var (
			trueHead = request.Block.ParentHash()
			trueTD   = new(big.Int).Sub(request.TD, request.Block.Difficulty())
		)
		
		if _, td := p.Head(); trueTD.Cmp(td) > 0 {
			p.SetHead(trueHead, trueTD)
			pm.chainSync.handlePeerEvent(p)
		}

	case msg.Code == NewPooledTransactionHashesMsg && p.version >= eth65:
		
		
		if atomic.LoadUint32(&pm.acceptTxs) == 0 {
			break
		}
		var hashes []common.Hash
		if err := msg.Decode(&hashes); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		
		for _, hash := range hashes {
			p.MarkTransaction(hash)
		}
		pm.txFetcher.Notify(p.id, hashes)

	case msg.Code == GetPooledTransactionsMsg && p.version >= eth65:
		
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		
		var (
			hash   common.Hash
			bytes  int
			hashes []common.Hash
			txs    []rlp.RawValue
		)
		for bytes < softResponseLimit {
			
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			
			tx := pm.txpool.Get(hash)
			if tx == nil {
				continue
			}
			
			if encoded, err := rlp.EncodeToBytes(tx); err != nil {
				log.Error("Failed to encode transaction", "err", err)
			} else {
				hashes = append(hashes, hash)
				txs = append(txs, encoded)
				bytes += len(encoded)
			}
		}
		return p.SendPooledTransactionsRLP(hashes, txs)

	case msg.Code == TransactionMsg || (msg.Code == PooledTransactionsMsg && p.version >= eth65):
		
		if atomic.LoadUint32(&pm.acceptTxs) == 0 {
			break
		}
		
		var txs []*types.Transaction
		if err := msg.Decode(&txs); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		for i, tx := range txs {
			
			if tx == nil {
				return errResp(ErrDecode, "transaction %d is nil", i)
			}
			p.MarkTransaction(tx.Hash())
		}
		pm.txFetcher.Enqueue(p.id, txs, msg.Code == PooledTransactionsMsg)

	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	return nil
}



func (pm *ProtocolManager) BroadcastBlock(block *types.Block, propagate bool) {
	hash := block.Hash()
	peers := pm.peers.PeersWithoutBlock(hash)

	
	if propagate {
		
		var td *big.Int
		if parent := pm.blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1); parent != nil {
			td = new(big.Int).Add(block.Difficulty(), pm.blockchain.GetTd(block.ParentHash(), block.NumberU64()-1))
		} else {
			log.Error("Propagating dangling block", "number", block.Number(), "hash", hash)
			return
		}
		
		transfer := peers[:int(math.Sqrt(float64(len(peers))))]
		for _, peer := range transfer {
			peer.AsyncSendNewBlock(block, td)
		}
		log.Trace("Propagated block", "hash", hash, "recipients", len(transfer), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
		return
	}
	
	if pm.blockchain.HasBlock(hash, block.NumberU64()) {
		for _, peer := range peers {
			peer.AsyncSendNewBlockHash(block)
		}
		log.Trace("Announced block", "hash", hash, "recipients", len(peers), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
	}
}



func (pm *ProtocolManager) BroadcastTransactions(txs types.Transactions, propagate bool) {
	var (
		txset = make(map[*peer][]common.Hash)
		annos = make(map[*peer][]common.Hash)
	)
	
	if propagate {
		for _, tx := range txs {
			peers := pm.peers.PeersWithoutTx(tx.Hash())

			
			transfer := peers[:int(math.Sqrt(float64(len(peers))))]
			for _, peer := range transfer {
				txset[peer] = append(txset[peer], tx.Hash())
			}
			log.Trace("Broadcast transaction", "hash", tx.Hash(), "recipients", len(peers))
		}
		for peer, hashes := range txset {
			peer.AsyncSendTransactions(hashes)
		}
		return
	}
	
	for _, tx := range txs {
		peers := pm.peers.PeersWithoutTx(tx.Hash())
		for _, peer := range peers {
			annos[peer] = append(annos[peer], tx.Hash())
		}
	}
	for peer, hashes := range annos {
		if peer.version >= eth65 {
			peer.AsyncSendPooledTransactionHashes(hashes)
		} else {
			peer.AsyncSendTransactions(hashes)
		}
	}
}


func (pm *ProtocolManager) minedBroadcastLoop() {
	defer pm.wg.Done()

	for obj := range pm.minedBlockSub.Chan() {
		if ev, ok := obj.Data.(core.NewMinedBlockEvent); ok {
			pm.BroadcastBlock(ev.Block, true)  
			pm.BroadcastBlock(ev.Block, false) 
		}
	}
}


func (pm *ProtocolManager) txBroadcastLoop() {
	defer pm.wg.Done()

	for {
		select {
		case event := <-pm.txsCh:
			
			if pm.broadcastTxAnnouncesOnly {
				pm.BroadcastTransactions(event.Txs, false)
				continue
			}
			pm.BroadcastTransactions(event.Txs, true)  
			pm.BroadcastTransactions(event.Txs, false) 

		case <-pm.txsSub.Err():
			return
		}
	}
}



type NodeInfo struct {
	Network    uint64              `json:"network"`    
	Difficulty *big.Int            `json:"difficulty"` 
	Genesis    common.Hash         `json:"genesis"`    
	Config     *params.ChainConfig `json:"config"`     
	Head       common.Hash         `json:"head"`       
}


func (pm *ProtocolManager) NodeInfo() *NodeInfo {
	currentBlock := pm.blockchain.CurrentBlock()
	return &NodeInfo{
		Network:    pm.networkID,
		Difficulty: pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64()),
		Genesis:    pm.blockchain.Genesis().Hash(),
		Config:     pm.blockchain.Config(),
		Head:       currentBlock.Hash(),
	}
}
