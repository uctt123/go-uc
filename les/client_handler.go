















package les

import (
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/params"
)



type clientHandler struct {
	ulc        *ulc
	checkpoint *params.TrustedCheckpoint
	fetcher    *lightFetcher
	downloader *downloader.Downloader
	backend    *LightEthereum

	closeCh  chan struct{}
	wg       sync.WaitGroup 
	syncDone func()         
}

func newClientHandler(ulcServers []string, ulcFraction int, checkpoint *params.TrustedCheckpoint, backend *LightEthereum) *clientHandler {
	handler := &clientHandler{
		checkpoint: checkpoint,
		backend:    backend,
		closeCh:    make(chan struct{}),
	}
	if ulcServers != nil {
		ulc, err := newULC(ulcServers, ulcFraction)
		if err != nil {
			log.Error("Failed to initialize ultra light client")
		}
		handler.ulc = ulc
		log.Info("Enable ultra light client mode")
	}
	var height uint64
	if checkpoint != nil {
		height = (checkpoint.SectionIndex+1)*params.CHTFrequency - 1
	}
	handler.fetcher = newLightFetcher(backend.blockchain, backend.engine, backend.peers, handler.ulc, backend.chainDb, backend.reqDist, handler.synchronise)
	handler.downloader = downloader.New(height, backend.chainDb, nil, backend.eventMux, nil, backend.blockchain, handler.removePeer)
	handler.backend.peers.subscribe((*downloaderPeerNotify)(handler))
	return handler
}

func (h *clientHandler) start() {
	h.fetcher.start()
}

func (h *clientHandler) stop() {
	close(h.closeCh)
	h.downloader.Terminate()
	h.fetcher.stop()
	h.wg.Wait()
}


func (h *clientHandler) runPeer(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) error {
	trusted := false
	if h.ulc != nil {
		trusted = h.ulc.trusted(p.ID())
	}
	peer := newServerPeer(int(version), h.backend.config.NetworkId, trusted, p, newMeteredMsgWriter(rw, int(version)))
	defer peer.close()
	h.wg.Add(1)
	defer h.wg.Done()
	err := h.handle(peer)
	return err
}

func (h *clientHandler) handle(p *serverPeer) error {
	if h.backend.peers.len() >= h.backend.config.LightPeers && !p.Peer.Info().Network.Trusted {
		return p2p.DiscTooManyPeers
	}
	p.Log().Debug("Light Ethereum peer connected", "name", p.Name())

	
	var (
		head   = h.backend.blockchain.CurrentHeader()
		hash   = head.Hash()
		number = head.Number.Uint64()
		td     = h.backend.blockchain.GetTd(hash, number)
	)
	if err := p.Handshake(td, hash, number, h.backend.blockchain.Genesis().Hash(), nil); err != nil {
		p.Log().Debug("Light Ethereum handshake failed", "err", err)
		return err
	}
	
	if err := h.backend.peers.register(p); err != nil {
		p.Log().Error("Light Ethereum peer registration failed", "err", err)
		return err
	}
	serverConnectionGauge.Update(int64(h.backend.peers.len()))

	connectedAt := mclock.Now()
	defer func() {
		h.backend.peers.unregister(p.id)
		connectionTimer.Update(time.Duration(mclock.Now() - connectedAt))
		serverConnectionGauge.Update(int64(h.backend.peers.len()))
	}()
	h.fetcher.announce(p, &announceData{Hash: p.headInfo.Hash, Number: p.headInfo.Number, Td: p.headInfo.Td})

	
	atomic.StoreUint32(&p.serving, 1)
	defer atomic.StoreUint32(&p.serving, 0)

	
	for {
		if err := h.handleMsg(p); err != nil {
			p.Log().Debug("Light Ethereum message handling failed", "err", err)
			p.fcServer.DumpLogs()
			return err
		}
	}
}



func (h *clientHandler) handleMsg(p *serverPeer) error {
	
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	p.Log().Trace("Light Ethereum message arrived", "code", msg.Code, "bytes", msg.Size)

	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	var deliverMsg *Msg

	
	switch msg.Code {
	case AnnounceMsg:
		p.Log().Trace("Received announce message")
		var req announceData
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		if err := req.sanityCheck(); err != nil {
			return err
		}
		update, size := req.Update.decode()
		if p.rejectUpdate(size) {
			return errResp(ErrRequestRejected, "")
		}
		p.updateFlowControl(update)
		p.updateVtParams()

		if req.Hash != (common.Hash{}) {
			if p.announceType == announceTypeNone {
				return errResp(ErrUnexpectedResponse, "")
			}
			if p.announceType == announceTypeSigned {
				if err := req.checkSignature(p.ID(), update); err != nil {
					p.Log().Trace("Invalid announcement signature", "err", err)
					return err
				}
				p.Log().Trace("Valid announcement signature")
			}
			p.Log().Trace("Announce message content", "number", req.Number, "hash", req.Hash, "td", req.Td, "reorg", req.ReorgDepth)

			
			p.updateHead(req.Hash, req.Number, req.Td)
			h.fetcher.announce(p, &req)
		}
	case BlockHeadersMsg:
		p.Log().Trace("Received block header response message")
		var resp struct {
			ReqID, BV uint64
			Headers   []*types.Header
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		headers := resp.Headers
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)

		
		filter := len(headers) == 1
		if filter {
			headers = h.fetcher.deliverHeaders(p, resp.ReqID, resp.Headers)
		}
		if len(headers) != 0 || !filter {
			if err := h.downloader.DeliverHeaders(p.id, headers); err != nil {
				log.Debug("Failed to deliver headers", "err", err)
			}
		}
	case BlockBodiesMsg:
		p.Log().Trace("Received block bodies response")
		var resp struct {
			ReqID, BV uint64
			Data      []*types.Body
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgBlockBodies,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case CodeMsg:
		p.Log().Trace("Received code response")
		var resp struct {
			ReqID, BV uint64
			Data      [][]byte
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgCode,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case ReceiptsMsg:
		p.Log().Trace("Received receipts response")
		var resp struct {
			ReqID, BV uint64
			Receipts  []types.Receipts
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgReceipts,
			ReqID:   resp.ReqID,
			Obj:     resp.Receipts,
		}
	case ProofsV2Msg:
		p.Log().Trace("Received les/2 proofs response")
		var resp struct {
			ReqID, BV uint64
			Data      light.NodeList
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgProofsV2,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case HelperTrieProofsMsg:
		p.Log().Trace("Received helper trie proof response")
		var resp struct {
			ReqID, BV uint64
			Data      HelperTrieResps
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgHelperTrieProofs,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case TxStatusMsg:
		p.Log().Trace("Received tx status response")
		var resp struct {
			ReqID, BV uint64
			Status    []light.TxStatus
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgTxStatus,
			ReqID:   resp.ReqID,
			Obj:     resp.Status,
		}
	case StopMsg:
		p.freeze()
		h.backend.retriever.frozen(p)
		p.Log().Debug("Service stopped")
	case ResumeMsg:
		var bv uint64
		if err := msg.Decode(&bv); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ResumeFreeze(bv)
		p.unfreeze()
		p.Log().Debug("Service resumed")
	default:
		p.Log().Trace("Received invalid message", "code", msg.Code)
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	
	if deliverMsg != nil {
		if err := h.backend.retriever.deliver(p, deliverMsg); err != nil {
			if val := p.errCount.Add(1, mclock.Now()); val > maxResponseErrors {
				return err
			}
		}
	}
	return nil
}

func (h *clientHandler) removePeer(id string) {
	h.backend.peers.unregister(id)
}

type peerConnection struct {
	handler *clientHandler
	peer    *serverPeer
}

func (pc *peerConnection) Head() (common.Hash, *big.Int) {
	return pc.peer.HeadAndTd()
}

func (pc *peerConnection) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*serverPeer)
			return peer.getRequestCost(GetBlockHeadersMsg, amount)
		},
		canSend: func(dp distPeer) bool {
			return dp.(*serverPeer) == pc.peer
		},
		request: func(dp distPeer) func() {
			reqID := genReqID()
			peer := dp.(*serverPeer)
			cost := peer.getRequestCost(GetBlockHeadersMsg, amount)
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.requestHeadersByHash(reqID, origin, amount, skip, reverse) }
		},
	}
	_, ok := <-pc.handler.backend.reqDist.queue(rq)
	if !ok {
		return light.ErrNoPeers
	}
	return nil
}

func (pc *peerConnection) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool) error {
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*serverPeer)
			return peer.getRequestCost(GetBlockHeadersMsg, amount)
		},
		canSend: func(dp distPeer) bool {
			return dp.(*serverPeer) == pc.peer
		},
		request: func(dp distPeer) func() {
			reqID := genReqID()
			peer := dp.(*serverPeer)
			cost := peer.getRequestCost(GetBlockHeadersMsg, amount)
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.requestHeadersByNumber(reqID, origin, amount, skip, reverse) }
		},
	}
	_, ok := <-pc.handler.backend.reqDist.queue(rq)
	if !ok {
		return light.ErrNoPeers
	}
	return nil
}


type downloaderPeerNotify clientHandler

func (d *downloaderPeerNotify) registerPeer(p *serverPeer) {
	h := (*clientHandler)(d)
	pc := &peerConnection{
		handler: h,
		peer:    p,
	}
	h.downloader.RegisterLightPeer(p.id, ethVersion, pc)
}

func (d *downloaderPeerNotify) unregisterPeer(p *serverPeer) {
	h := (*clientHandler)(d)
	h.downloader.UnregisterPeer(p.id)
}
