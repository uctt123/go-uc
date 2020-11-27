















package eth

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

const (
	maxKnownTxs    = 32768 
	maxKnownBlocks = 1024  

	
	
	maxQueuedTxs = 4096

	
	
	maxQueuedTxAnns = 4096

	
	
	
	maxQueuedBlocks = 4

	
	
	
	maxQueuedBlockAnns = 4

	handshakeTimeout = 5 * time.Second
)


func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}



type PeerInfo struct {
	Version    int      `json:"version"`    
	Difficulty *big.Int `json:"difficulty"` 
	Head       string   `json:"head"`       
}


type propEvent struct {
	block *types.Block
	td    *big.Int
}

type peer struct {
	id string

	*p2p.Peer
	rw p2p.MsgReadWriter

	version  int         
	syncDrop *time.Timer 

	head common.Hash
	td   *big.Int
	lock sync.RWMutex

	knownBlocks     mapset.Set        
	queuedBlocks    chan *propEvent   
	queuedBlockAnns chan *types.Block 

	knownTxs    mapset.Set                           
	txBroadcast chan []common.Hash                   
	txAnnounce  chan []common.Hash                   
	getPooledTx func(common.Hash) *types.Transaction 

	term chan struct{} 
}

func newPeer(version int, p *p2p.Peer, rw p2p.MsgReadWriter, getPooledTx func(hash common.Hash) *types.Transaction) *peer {
	return &peer{
		Peer:            p,
		rw:              rw,
		version:         version,
		id:              fmt.Sprintf("%x", p.ID().Bytes()[:8]),
		knownTxs:        mapset.NewSet(),
		knownBlocks:     mapset.NewSet(),
		queuedBlocks:    make(chan *propEvent, maxQueuedBlocks),
		queuedBlockAnns: make(chan *types.Block, maxQueuedBlockAnns),
		txBroadcast:     make(chan []common.Hash),
		txAnnounce:      make(chan []common.Hash),
		getPooledTx:     getPooledTx,
		term:            make(chan struct{}),
	}
}




func (p *peer) broadcastBlocks(removePeer func(string)) {
	for {
		select {
		case prop := <-p.queuedBlocks:
			if err := p.SendNewBlock(prop.block, prop.td); err != nil {
				removePeer(p.id)
				return
			}
			p.Log().Trace("Propagated block", "number", prop.block.Number(), "hash", prop.block.Hash(), "td", prop.td)

		case block := <-p.queuedBlockAnns:
			if err := p.SendNewBlockHashes([]common.Hash{block.Hash()}, []uint64{block.NumberU64()}); err != nil {
				removePeer(p.id)
				return
			}
			p.Log().Trace("Announced block", "number", block.Number(), "hash", block.Hash())

		case <-p.term:
			return
		}
	}
}




func (p *peer) broadcastTransactions(removePeer func(string)) {
	var (
		queue []common.Hash         
		done  chan struct{}         
		fail  = make(chan error, 1) 
	)
	for {
		
		if done == nil && len(queue) > 0 {
			
			var (
				hashes []common.Hash
				txs    []*types.Transaction
				size   common.StorageSize
			)
			for i := 0; i < len(queue) && size < txsyncPackSize; i++ {
				if tx := p.getPooledTx(queue[i]); tx != nil {
					txs = append(txs, tx)
					size += tx.Size()
				}
				hashes = append(hashes, queue[i])
			}
			queue = queue[:copy(queue, queue[len(hashes):])]

			
			if len(txs) > 0 {
				done = make(chan struct{})
				go func() {
					if err := p.sendTransactions(txs); err != nil {
						fail <- err
						return
					}
					close(done)
					p.Log().Trace("Sent transactions", "count", len(txs))
				}()
			}
		}
		
		select {
		case hashes := <-p.txBroadcast:
			
			queue = append(queue, hashes...)
			if len(queue) > maxQueuedTxs {
				
				queue = queue[:copy(queue, queue[len(queue)-maxQueuedTxs:])]
			}

		case <-done:
			done = nil

		case <-fail:
			removePeer(p.id)
			return

		case <-p.term:
			return
		}
	}
}




func (p *peer) announceTransactions(removePeer func(string)) {
	var (
		queue []common.Hash         
		done  chan struct{}         
		fail  = make(chan error, 1) 
	)
	for {
		
		if done == nil && len(queue) > 0 {
			
			var (
				hashes  []common.Hash
				pending []common.Hash
				size    common.StorageSize
			)
			for i := 0; i < len(queue) && size < txsyncPackSize; i++ {
				if p.getPooledTx(queue[i]) != nil {
					pending = append(pending, queue[i])
					size += common.HashLength
				}
				hashes = append(hashes, queue[i])
			}
			queue = queue[:copy(queue, queue[len(hashes):])]

			
			if len(pending) > 0 {
				done = make(chan struct{})
				go func() {
					if err := p.sendPooledTransactionHashes(pending); err != nil {
						fail <- err
						return
					}
					close(done)
					p.Log().Trace("Sent transaction announcements", "count", len(pending))
				}()
			}
		}
		
		select {
		case hashes := <-p.txAnnounce:
			
			queue = append(queue, hashes...)
			if len(queue) > maxQueuedTxAnns {
				
				queue = queue[:copy(queue, queue[len(queue)-maxQueuedTxAnns:])]
			}

		case <-done:
			done = nil

		case <-fail:
			removePeer(p.id)
			return

		case <-p.term:
			return
		}
	}
}


func (p *peer) close() {
	close(p.term)
}


func (p *peer) Info() *PeerInfo {
	hash, td := p.Head()

	return &PeerInfo{
		Version:    p.version,
		Difficulty: td,
		Head:       hash.Hex(),
	}
}



func (p *peer) Head() (hash common.Hash, td *big.Int) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	copy(hash[:], p.head[:])
	return hash, new(big.Int).Set(p.td)
}


func (p *peer) SetHead(hash common.Hash, td *big.Int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	copy(p.head[:], hash[:])
	p.td.Set(td)
}



func (p *peer) MarkBlock(hash common.Hash) {
	
	for p.knownBlocks.Cardinality() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(hash)
}



func (p *peer) MarkTransaction(hash common.Hash) {
	
	for p.knownTxs.Cardinality() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(hash)
}






func (p *peer) SendTransactions64(txs types.Transactions) error {
	return p.sendTransactions(txs)
}







func (p *peer) sendTransactions(txs types.Transactions) error {
	
	for p.knownTxs.Cardinality() > max(0, maxKnownTxs-len(txs)) {
		p.knownTxs.Pop()
	}
	for _, tx := range txs {
		p.knownTxs.Add(tx.Hash())
	}
	return p2p.Send(p.rw, TransactionMsg, txs)
}




func (p *peer) AsyncSendTransactions(hashes []common.Hash) {
	select {
	case p.txBroadcast <- hashes:
		
		for p.knownTxs.Cardinality() > max(0, maxKnownTxs-len(hashes)) {
			p.knownTxs.Pop()
		}
		for _, hash := range hashes {
			p.knownTxs.Add(hash)
		}
	case <-p.term:
		p.Log().Debug("Dropping transaction propagation", "count", len(hashes))
	}
}







func (p *peer) sendPooledTransactionHashes(hashes []common.Hash) error {
	
	for p.knownTxs.Cardinality() > max(0, maxKnownTxs-len(hashes)) {
		p.knownTxs.Pop()
	}
	for _, hash := range hashes {
		p.knownTxs.Add(hash)
	}
	return p2p.Send(p.rw, NewPooledTransactionHashesMsg, hashes)
}




func (p *peer) AsyncSendPooledTransactionHashes(hashes []common.Hash) {
	select {
	case p.txAnnounce <- hashes:
		
		for p.knownTxs.Cardinality() > max(0, maxKnownTxs-len(hashes)) {
			p.knownTxs.Pop()
		}
		for _, hash := range hashes {
			p.knownTxs.Add(hash)
		}
	case <-p.term:
		p.Log().Debug("Dropping transaction announcement", "count", len(hashes))
	}
}






func (p *peer) SendPooledTransactionsRLP(hashes []common.Hash, txs []rlp.RawValue) error {
	
	for p.knownTxs.Cardinality() > max(0, maxKnownTxs-len(hashes)) {
		p.knownTxs.Pop()
	}
	for _, hash := range hashes {
		p.knownTxs.Add(hash)
	}
	return p2p.Send(p.rw, PooledTransactionsMsg, txs)
}



func (p *peer) SendNewBlockHashes(hashes []common.Hash, numbers []uint64) error {
	
	for p.knownBlocks.Cardinality() > max(0, maxKnownBlocks-len(hashes)) {
		p.knownBlocks.Pop()
	}
	for _, hash := range hashes {
		p.knownBlocks.Add(hash)
	}
	request := make(newBlockHashesData, len(hashes))
	for i := 0; i < len(hashes); i++ {
		request[i].Hash = hashes[i]
		request[i].Number = numbers[i]
	}
	return p2p.Send(p.rw, NewBlockHashesMsg, request)
}




func (p *peer) AsyncSendNewBlockHash(block *types.Block) {
	select {
	case p.queuedBlockAnns <- block:
		
		for p.knownBlocks.Cardinality() >= maxKnownBlocks {
			p.knownBlocks.Pop()
		}
		p.knownBlocks.Add(block.Hash())
	default:
		p.Log().Debug("Dropping block announcement", "number", block.NumberU64(), "hash", block.Hash())
	}
}


func (p *peer) SendNewBlock(block *types.Block, td *big.Int) error {
	
	for p.knownBlocks.Cardinality() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(block.Hash())
	return p2p.Send(p.rw, NewBlockMsg, []interface{}{block, td})
}



func (p *peer) AsyncSendNewBlock(block *types.Block, td *big.Int) {
	select {
	case p.queuedBlocks <- &propEvent{block: block, td: td}:
		
		for p.knownBlocks.Cardinality() >= maxKnownBlocks {
			p.knownBlocks.Pop()
		}
		p.knownBlocks.Add(block.Hash())
	default:
		p.Log().Debug("Dropping block propagation", "number", block.NumberU64(), "hash", block.Hash())
	}
}


func (p *peer) SendBlockHeaders(headers []*types.Header) error {
	return p2p.Send(p.rw, BlockHeadersMsg, headers)
}


func (p *peer) SendBlockBodies(bodies []*blockBody) error {
	return p2p.Send(p.rw, BlockBodiesMsg, blockBodiesData(bodies))
}



func (p *peer) SendBlockBodiesRLP(bodies []rlp.RawValue) error {
	return p2p.Send(p.rw, BlockBodiesMsg, bodies)
}



func (p *peer) SendNodeData(data [][]byte) error {
	return p2p.Send(p.rw, NodeDataMsg, data)
}



func (p *peer) SendReceiptsRLP(receipts []rlp.RawValue) error {
	return p2p.Send(p.rw, ReceiptsMsg, receipts)
}



func (p *peer) RequestOneHeader(hash common.Hash) error {
	p.Log().Debug("Fetching single header", "hash", hash)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false})
}



func (p *peer) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromhash", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}



func (p *peer) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}



func (p *peer) RequestBodies(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of block bodies", "count", len(hashes))
	return p2p.Send(p.rw, GetBlockBodiesMsg, hashes)
}



func (p *peer) RequestNodeData(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of state data", "count", len(hashes))
	return p2p.Send(p.rw, GetNodeDataMsg, hashes)
}


func (p *peer) RequestReceipts(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of receipts", "count", len(hashes))
	return p2p.Send(p.rw, GetReceiptsMsg, hashes)
}


func (p *peer) RequestTxs(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of transactions", "count", len(hashes))
	return p2p.Send(p.rw, GetPooledTransactionsMsg, hashes)
}



func (p *peer) Handshake(network uint64, td *big.Int, head common.Hash, genesis common.Hash, forkID forkid.ID, forkFilter forkid.Filter) error {
	
	errc := make(chan error, 2)

	var (
		status63 statusData63 
		status   statusData   
	)
	go func() {
		switch {
		case p.version == eth63:
			errc <- p2p.Send(p.rw, StatusMsg, &statusData63{
				ProtocolVersion: uint32(p.version),
				NetworkId:       network,
				TD:              td,
				CurrentBlock:    head,
				GenesisBlock:    genesis,
			})
		case p.version >= eth64:
			errc <- p2p.Send(p.rw, StatusMsg, &statusData{
				ProtocolVersion: uint32(p.version),
				NetworkID:       network,
				TD:              td,
				Head:            head,
				Genesis:         genesis,
				ForkID:          forkID,
			})
		default:
			panic(fmt.Sprintf("unsupported eth protocol version: %d", p.version))
		}
	}()
	go func() {
		switch {
		case p.version == eth63:
			errc <- p.readStatusLegacy(network, &status63, genesis)
		case p.version >= eth64:
			errc <- p.readStatus(network, &status, genesis, forkFilter)
		default:
			panic(fmt.Sprintf("unsupported eth protocol version: %d", p.version))
		}
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return err
			}
		case <-timeout.C:
			return p2p.DiscReadTimeout
		}
	}
	switch {
	case p.version == eth63:
		p.td, p.head = status63.TD, status63.CurrentBlock
	case p.version >= eth64:
		p.td, p.head = status.TD, status.Head
	default:
		panic(fmt.Sprintf("unsupported eth protocol version: %d", p.version))
	}
	return nil
}

func (p *peer) readStatusLegacy(network uint64, status *statusData63, genesis common.Hash) error {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != StatusMsg {
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, StatusMsg)
	}
	if msg.Size > protocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, protocolMaxMsgSize)
	}
	
	if err := msg.Decode(&status); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}
	if status.GenesisBlock != genesis {
		return errResp(ErrGenesisMismatch, "%x (!= %x)", status.GenesisBlock[:8], genesis[:8])
	}
	if status.NetworkId != network {
		return errResp(ErrNetworkIDMismatch, "%d (!= %d)", status.NetworkId, network)
	}
	if int(status.ProtocolVersion) != p.version {
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, p.version)
	}
	return nil
}

func (p *peer) readStatus(network uint64, status *statusData, genesis common.Hash, forkFilter forkid.Filter) error {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != StatusMsg {
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, StatusMsg)
	}
	if msg.Size > protocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, protocolMaxMsgSize)
	}
	
	if err := msg.Decode(&status); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}
	if status.NetworkID != network {
		return errResp(ErrNetworkIDMismatch, "%d (!= %d)", status.NetworkID, network)
	}
	if int(status.ProtocolVersion) != p.version {
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, p.version)
	}
	if status.Genesis != genesis {
		return errResp(ErrGenesisMismatch, "%x (!= %x)", status.Genesis, genesis)
	}
	if err := forkFilter(status.ForkID); err != nil {
		return errResp(ErrForkIDRejected, "%v", err)
	}
	return nil
}


func (p *peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.id,
		fmt.Sprintf("eth/%2d", p.version),
	)
}



type peerSet struct {
	peers  map[string]*peer
	lock   sync.RWMutex
	closed bool
}


func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[string]*peer),
	}
}




func (ps *peerSet) Register(p *peer, removePeer func(string)) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers[p.id]; ok {
		return errAlreadyRegistered
	}
	ps.peers[p.id] = p

	go p.broadcastBlocks(removePeer)
	go p.broadcastTransactions(removePeer)
	if p.version >= eth65 {
		go p.announceTransactions(removePeer)
	}
	return nil
}



func (ps *peerSet) Unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	p, ok := ps.peers[id]
	if !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)
	p.close()

	return nil
}


func (ps *peerSet) Peer(id string) *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}


func (ps *peerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}



func (ps *peerSet) PeersWithoutBlock(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownBlocks.Contains(hash) {
			list = append(list, p)
		}
	}
	return list
}



func (ps *peerSet) PeersWithoutTx(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownTxs.Contains(hash) {
			list = append(list, p)
		}
	}
	return list
}


func (ps *peerSet) BestPeer() *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *peer
		bestTd   *big.Int
	)
	for _, p := range ps.peers {
		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, td
		}
	}
	return bestPeer
}



func (ps *peerSet) Close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}
