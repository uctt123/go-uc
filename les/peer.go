















package les

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/les/flowcontrol"
	lpc "github.com/ethereum/go-ethereum/les/lespay/client"
	lps "github.com/ethereum/go-ethereum/les/lespay/server"
	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

const (
	maxRequestErrors  = 20 
	maxResponseErrors = 50 

	allowedUpdateBytes = 100000                
	allowedUpdateRate  = time.Millisecond * 10 

	freezeTimeBase    = time.Millisecond * 700 
	freezeTimeRandom  = time.Millisecond * 600 
	freezeCheckPeriod = time.Millisecond * 100 

	
	
	
	txSizeCostLimit = 0x4000

	
	handshakeTimeout = 5 * time.Second
)

const (
	announceTypeNone = iota
	announceTypeSimple
	announceTypeSigned
)

type keyValueEntry struct {
	Key   string
	Value rlp.RawValue
}

type keyValueList []keyValueEntry
type keyValueMap map[string]rlp.RawValue

func (l keyValueList) add(key string, val interface{}) keyValueList {
	var entry keyValueEntry
	entry.Key = key
	if val == nil {
		val = uint64(0)
	}
	enc, err := rlp.EncodeToBytes(val)
	if err == nil {
		entry.Value = enc
	}
	return append(l, entry)
}

func (l keyValueList) decode() (keyValueMap, uint64) {
	m := make(keyValueMap)
	var size uint64
	for _, entry := range l {
		m[entry.Key] = entry.Value
		size += uint64(len(entry.Key)) + uint64(len(entry.Value)) + 8
	}
	return m, size
}

func (m keyValueMap) get(key string, val interface{}) error {
	enc, ok := m[key]
	if !ok {
		return errResp(ErrMissingKey, "%s", key)
	}
	if val == nil {
		return nil
	}
	return rlp.DecodeBytes(enc, val)
}


type peerCommons struct {
	*p2p.Peer
	rw p2p.MsgReadWriter

	id           string    
	version      int       
	network      uint64    
	frozen       uint32    
	announceType uint64    
	serving      uint32    
	headInfo     blockInfo 

	
	sendQueue *utils.ExecQueue

	
	fcParams flowcontrol.ServerParams 
	fcCosts  requestCostTable         

	closeCh chan struct{}
	lock    sync.RWMutex 
}



func (p *peerCommons) isFrozen() bool {
	return atomic.LoadUint32(&p.frozen) != 0
}


func (p *peerCommons) canQueue() bool {
	return p.sendQueue.CanQueue() && !p.isFrozen()
}



func (p *peerCommons) queueSend(f func()) bool {
	return p.sendQueue.Queue(f)
}


func (p *peerCommons) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.id, fmt.Sprintf("les/%d", p.version))
}


func (p *peerCommons) Info() *eth.PeerInfo {
	return &eth.PeerInfo{
		Version:    p.version,
		Difficulty: p.Td(),
		Head:       fmt.Sprintf("%x", p.Head()),
	}
}


func (p *peerCommons) Head() (hash common.Hash) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.headInfo.Hash
}


func (p *peerCommons) Td() *big.Int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return new(big.Int).Set(p.headInfo.Td)
}


func (p *peerCommons) HeadAndTd() (hash common.Hash, td *big.Int) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.headInfo.Hash, new(big.Int).Set(p.headInfo.Td)
}



func (p *peerCommons) sendReceiveHandshake(sendList keyValueList) (keyValueList, error) {
	var (
		errc     = make(chan error, 2)
		recvList keyValueList
	)
	
	go func() {
		errc <- p2p.Send(p.rw, StatusMsg, sendList)
	}()
	go func() {
		
		msg, err := p.rw.ReadMsg()
		if err != nil {
			errc <- err
			return
		}
		if msg.Code != StatusMsg {
			errc <- errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, StatusMsg)
			return
		}
		if msg.Size > ProtocolMaxMsgSize {
			errc <- errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
			return
		}
		
		if err := msg.Decode(&recvList); err != nil {
			errc <- errResp(ErrDecode, "msg %v: %v", msg, err)
			return
		}
		errc <- nil
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return nil, err
			}
		case <-timeout.C:
			return nil, p2p.DiscReadTimeout
		}
	}
	return recvList, nil
}





func (p *peerCommons) handshake(td *big.Int, head common.Hash, headNum uint64, genesis common.Hash, sendCallback func(*keyValueList), recvCallback func(keyValueMap) error) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	var send keyValueList

	
	send = send.add("protocolVersion", uint64(p.version))
	send = send.add("networkId", p.network)
	send = send.add("headTd", td)
	send = send.add("headHash", head)
	send = send.add("headNum", headNum)
	send = send.add("genesisHash", genesis)

	
	if sendCallback != nil {
		sendCallback(&send)
	}
	
	recvList, err := p.sendReceiveHandshake(send)
	if err != nil {
		return err
	}
	recv, size := recvList.decode()
	if size > allowedUpdateBytes {
		return errResp(ErrRequestRejected, "")
	}
	var rGenesis, rHash common.Hash
	var rVersion, rNetwork, rNum uint64
	var rTd *big.Int
	if err := recv.get("protocolVersion", &rVersion); err != nil {
		return err
	}
	if err := recv.get("networkId", &rNetwork); err != nil {
		return err
	}
	if err := recv.get("headTd", &rTd); err != nil {
		return err
	}
	if err := recv.get("headHash", &rHash); err != nil {
		return err
	}
	if err := recv.get("headNum", &rNum); err != nil {
		return err
	}
	if err := recv.get("genesisHash", &rGenesis); err != nil {
		return err
	}
	if rGenesis != genesis {
		return errResp(ErrGenesisBlockMismatch, "%x (!= %x)", rGenesis[:8], genesis[:8])
	}
	if rNetwork != p.network {
		return errResp(ErrNetworkIdMismatch, "%d (!= %d)", rNetwork, p.network)
	}
	if int(rVersion) != p.version {
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", rVersion, p.version)
	}
	p.headInfo = blockInfo{Hash: rHash, Number: rNum, Td: rTd}
	if recvCallback != nil {
		return recvCallback(recv)
	}
	return nil
}


func (p *peerCommons) close() {
	close(p.closeCh)
	p.sendQueue.Quit()
}



type serverPeer struct {
	peerCommons

	
	trusted                 bool   
	onlyAnnounce            bool   
	chainSince, chainRecent uint64 
	stateSince, stateRecent uint64 

	
	checkpointNumber uint64                   
	checkpoint       params.TrustedCheckpoint 

	fcServer         *flowcontrol.ServerNode 
	vtLock           sync.Mutex
	valueTracker     *lpc.ValueTracker
	nodeValueTracker *lpc.NodeValueTracker
	sentReqs         map[uint64]sentReqEntry

	
	errCount    utils.LinearExpiredValue 
	updateCount uint64
	updateTime  mclock.AbsTime

	
	hasBlockHook func(common.Hash, uint64, bool) bool 
}

func newServerPeer(version int, network uint64, trusted bool, p *p2p.Peer, rw p2p.MsgReadWriter) *serverPeer {
	return &serverPeer{
		peerCommons: peerCommons{
			Peer:      p,
			rw:        rw,
			id:        p.ID().String(),
			version:   version,
			network:   network,
			sendQueue: utils.NewExecQueue(100),
			closeCh:   make(chan struct{}),
		},
		trusted:  trusted,
		errCount: utils.LinearExpiredValue{Rate: mclock.AbsTime(time.Hour)},
	}
}



func (p *serverPeer) rejectUpdate(size uint64) bool {
	now := mclock.Now()
	if p.updateCount == 0 {
		p.updateTime = now
	} else {
		dt := now - p.updateTime
		p.updateTime = now

		r := uint64(dt / mclock.AbsTime(allowedUpdateRate))
		if p.updateCount > r {
			p.updateCount -= r
		} else {
			p.updateCount = 0
		}
	}
	p.updateCount += size
	return p.updateCount > allowedUpdateBytes
}



func (p *serverPeer) freeze() {
	if atomic.CompareAndSwapUint32(&p.frozen, 0, 1) {
		p.sendQueue.Clear()
	}
}



func (p *serverPeer) unfreeze() {
	atomic.StoreUint32(&p.frozen, 0)
}



func sendRequest(w p2p.MsgWriter, msgcode, reqID uint64, data interface{}) error {
	type req struct {
		ReqID uint64
		Data  interface{}
	}
	return p2p.Send(w, msgcode, req{reqID, data})
}

func (p *serverPeer) sendRequest(msgcode, reqID uint64, data interface{}, amount int) error {
	p.sentRequest(reqID, uint32(msgcode), uint32(amount))
	return sendRequest(p.rw, msgcode, reqID, data)
}



func (p *serverPeer) requestHeadersByHash(reqID uint64, origin common.Hash, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromhash", origin, "skip", skip, "reverse", reverse)
	return p.sendRequest(GetBlockHeadersMsg, reqID, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse}, amount)
}



func (p *serverPeer) requestHeadersByNumber(reqID, origin uint64, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	return p.sendRequest(GetBlockHeadersMsg, reqID, &getBlockHeadersData{Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse}, amount)
}



func (p *serverPeer) requestBodies(reqID uint64, hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of block bodies", "count", len(hashes))
	return p.sendRequest(GetBlockBodiesMsg, reqID, hashes, len(hashes))
}



func (p *serverPeer) requestCode(reqID uint64, reqs []CodeReq) error {
	p.Log().Debug("Fetching batch of codes", "count", len(reqs))
	return p.sendRequest(GetCodeMsg, reqID, reqs, len(reqs))
}


func (p *serverPeer) requestReceipts(reqID uint64, hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of receipts", "count", len(hashes))
	return p.sendRequest(GetReceiptsMsg, reqID, hashes, len(hashes))
}


func (p *serverPeer) requestProofs(reqID uint64, reqs []ProofReq) error {
	p.Log().Debug("Fetching batch of proofs", "count", len(reqs))
	return p.sendRequest(GetProofsV2Msg, reqID, reqs, len(reqs))
}


func (p *serverPeer) requestHelperTrieProofs(reqID uint64, reqs []HelperTrieReq) error {
	p.Log().Debug("Fetching batch of HelperTrie proofs", "count", len(reqs))
	return p.sendRequest(GetHelperTrieProofsMsg, reqID, reqs, len(reqs))
}


func (p *serverPeer) requestTxStatus(reqID uint64, txHashes []common.Hash) error {
	p.Log().Debug("Requesting transaction status", "count", len(txHashes))
	return p.sendRequest(GetTxStatusMsg, reqID, txHashes, len(txHashes))
}


func (p *serverPeer) sendTxs(reqID uint64, amount int, txs rlp.RawValue) error {
	p.Log().Debug("Sending batch of transactions", "amount", amount, "size", len(txs))
	sizeFactor := (len(txs) + txSizeCostLimit/2) / txSizeCostLimit
	if sizeFactor > amount {
		amount = sizeFactor
	}
	return p.sendRequest(SendTxV2Msg, reqID, txs, amount)
}


func (p *serverPeer) waitBefore(maxCost uint64) (time.Duration, float64) {
	return p.fcServer.CanSend(maxCost)
}



func (p *serverPeer) getRequestCost(msgcode uint64, amount int) uint64 {
	p.lock.RLock()
	defer p.lock.RUnlock()

	costs := p.fcCosts[msgcode]
	if costs == nil {
		return 0
	}
	cost := costs.baseCost + costs.reqCost*uint64(amount)
	if cost > p.fcParams.BufLimit {
		cost = p.fcParams.BufLimit
	}
	return cost
}



func (p *serverPeer) getTxRelayCost(amount, size int) uint64 {
	p.lock.RLock()
	defer p.lock.RUnlock()

	costs := p.fcCosts[SendTxV2Msg]
	if costs == nil {
		return 0
	}
	cost := costs.baseCost + costs.reqCost*uint64(amount)
	sizeCost := costs.baseCost + costs.reqCost*uint64(size)/txSizeCostLimit
	if sizeCost > cost {
		cost = sizeCost
	}
	if cost > p.fcParams.BufLimit {
		cost = p.fcParams.BufLimit
	}
	return cost
}


func (p *serverPeer) HasBlock(hash common.Hash, number uint64, hasState bool) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.hasBlockHook != nil {
		return p.hasBlockHook(hash, number, hasState)
	}
	head := p.headInfo.Number
	var since, recent uint64
	if hasState {
		since = p.stateSince
		recent = p.stateRecent
	} else {
		since = p.chainSince
		recent = p.chainRecent
	}
	return head >= number && number >= since && (recent == 0 || number+recent+4 > head)
}



func (p *serverPeer) updateFlowControl(update keyValueMap) {
	p.lock.Lock()
	defer p.lock.Unlock()

	
	var params flowcontrol.ServerParams
	if update.get("flowControl/BL", &params.BufLimit) == nil && update.get("flowControl/MRR", &params.MinRecharge) == nil {
		
		p.fcParams = params
		p.fcServer.UpdateParams(params)
	}
	var MRC RequestCostList
	if update.get("flowControl/MRC", &MRC) == nil {
		costUpdate := MRC.decode(ProtocolLengths[uint(p.version)])
		for code, cost := range costUpdate {
			p.fcCosts[code] = cost
		}
	}
}



func (p *serverPeer) updateHead(hash common.Hash, number uint64, td *big.Int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.headInfo = blockInfo{Hash: hash, Number: number, Td: td}
}



func (p *serverPeer) Handshake(td *big.Int, head common.Hash, headNum uint64, genesis common.Hash, server *LesServer) error {
	return p.handshake(td, head, headNum, genesis, func(lists *keyValueList) {
		
		
		
		p.announceType = announceTypeSimple
		if p.trusted {
			p.announceType = announceTypeSigned
		}
		*lists = (*lists).add("announceType", p.announceType)
	}, func(recv keyValueMap) error {
		if recv.get("serveChainSince", &p.chainSince) != nil {
			p.onlyAnnounce = true
		}
		if recv.get("serveRecentChain", &p.chainRecent) != nil {
			p.chainRecent = 0
		}
		if recv.get("serveStateSince", &p.stateSince) != nil {
			p.onlyAnnounce = true
		}
		if recv.get("serveRecentState", &p.stateRecent) != nil {
			p.stateRecent = 0
		}
		if recv.get("txRelay", nil) != nil {
			p.onlyAnnounce = true
		}
		if p.onlyAnnounce && !p.trusted {
			return errResp(ErrUselessPeer, "peer cannot serve requests")
		}
		
		var sParams flowcontrol.ServerParams
		if err := recv.get("flowControl/BL", &sParams.BufLimit); err != nil {
			return err
		}
		if err := recv.get("flowControl/MRR", &sParams.MinRecharge); err != nil {
			return err
		}
		var MRC RequestCostList
		if err := recv.get("flowControl/MRC", &MRC); err != nil {
			return err
		}
		p.fcParams = sParams
		p.fcServer = flowcontrol.NewServerNode(sParams, &mclock.System{})
		p.fcCosts = MRC.decode(ProtocolLengths[uint(p.version)])

		recv.get("checkpoint/value", &p.checkpoint)
		recv.get("checkpoint/registerHeight", &p.checkpointNumber)

		if !p.onlyAnnounce {
			for msgCode := range reqAvgTimeCost {
				if p.fcCosts[msgCode] == nil {
					return errResp(ErrUselessPeer, "peer does not support message %d", msgCode)
				}
			}
		}
		return nil
	})
}



func (p *serverPeer) setValueTracker(vt *lpc.ValueTracker, nvt *lpc.NodeValueTracker) {
	p.vtLock.Lock()
	p.valueTracker = vt
	p.nodeValueTracker = nvt
	if nvt != nil {
		p.sentReqs = make(map[uint64]sentReqEntry)
	} else {
		p.sentReqs = nil
	}
	p.vtLock.Unlock()
}


func (p *serverPeer) updateVtParams() {
	p.vtLock.Lock()
	defer p.vtLock.Unlock()

	if p.nodeValueTracker == nil {
		return
	}
	reqCosts := make([]uint64, len(requestList))
	for code, costs := range p.fcCosts {
		if m, ok := requestMapping[uint32(code)]; ok {
			reqCosts[m.first] = costs.baseCost + costs.reqCost
			if m.rest != -1 {
				reqCosts[m.rest] = costs.reqCost
			}
		}
	}
	p.valueTracker.UpdateCosts(p.nodeValueTracker, reqCosts)
}


type sentReqEntry struct {
	reqType, amount uint32
	at              mclock.AbsTime
}


func (p *serverPeer) sentRequest(id uint64, reqType, amount uint32) {
	p.vtLock.Lock()
	if p.sentReqs != nil {
		p.sentReqs[id] = sentReqEntry{reqType, amount, mclock.Now()}
	}
	p.vtLock.Unlock()
}


func (p *serverPeer) answeredRequest(id uint64) {
	p.vtLock.Lock()
	if p.sentReqs == nil {
		p.vtLock.Unlock()
		return
	}
	e, ok := p.sentReqs[id]
	delete(p.sentReqs, id)
	vt := p.valueTracker
	nvt := p.nodeValueTracker
	p.vtLock.Unlock()
	if !ok {
		return
	}
	var (
		vtReqs   [2]lpc.ServedRequest
		reqCount int
	)
	m := requestMapping[e.reqType]
	if m.rest == -1 || e.amount <= 1 {
		reqCount = 1
		vtReqs[0] = lpc.ServedRequest{ReqType: uint32(m.first), Amount: e.amount}
	} else {
		reqCount = 2
		vtReqs[0] = lpc.ServedRequest{ReqType: uint32(m.first), Amount: 1}
		vtReqs[1] = lpc.ServedRequest{ReqType: uint32(m.rest), Amount: e.amount - 1}
	}
	dt := time.Duration(mclock.Now() - e.at)
	vt.Served(nvt, vtReqs[:reqCount], dt)
}



type clientPeer struct {
	peerCommons

	
	
	responseLock  sync.Mutex
	responseCount uint64 

	balance *lps.NodeBalance

	
	invalidLock  sync.RWMutex
	invalidCount utils.LinearExpiredValue 

	server   bool
	errCh    chan error
	fcClient *flowcontrol.ClientNode 
}

func newClientPeer(version int, network uint64, p *p2p.Peer, rw p2p.MsgReadWriter) *clientPeer {
	return &clientPeer{
		peerCommons: peerCommons{
			Peer:      p,
			rw:        rw,
			id:        p.ID().String(),
			version:   version,
			network:   network,
			sendQueue: utils.NewExecQueue(100),
			closeCh:   make(chan struct{}),
		},
		invalidCount: utils.LinearExpiredValue{Rate: mclock.AbsTime(time.Hour)},
		errCh:        make(chan error, 1),
	}
}



func (p *clientPeer) freeClientId() string {
	if addr, ok := p.RemoteAddr().(*net.TCPAddr); ok {
		if addr.IP.IsLoopback() {
			
			
			return p.id
		} else {
			return addr.IP.String()
		}
	}
	return p.id
}


func (p *clientPeer) sendStop() error {
	return p2p.Send(p.rw, StopMsg, struct{}{})
}


func (p *clientPeer) sendResume(bv uint64) error {
	return p2p.Send(p.rw, ResumeMsg, bv)
}





func (p *clientPeer) freeze() {
	if p.version < lpv3 {
		
		
		atomic.StoreUint32(&p.frozen, 1)
		p.Peer.Disconnect(p2p.DiscUselessPeer)
		return
	}
	if atomic.SwapUint32(&p.frozen, 1) == 0 {
		go func() {
			p.sendStop()
			time.Sleep(freezeTimeBase + time.Duration(rand.Int63n(int64(freezeTimeRandom))))
			for {
				bufValue, bufLimit := p.fcClient.BufferStatus()
				if bufLimit == 0 {
					return
				}
				if bufValue <= bufLimit/8 {
					time.Sleep(freezeCheckPeriod)
					continue
				}
				atomic.StoreUint32(&p.frozen, 0)
				p.sendResume(bufValue)
				return
			}
		}()
	}
}




type reply struct {
	w              p2p.MsgWriter
	msgcode, reqID uint64
	data           rlp.RawValue
}


func (r *reply) send(bv uint64) error {
	type resp struct {
		ReqID, BV uint64
		Data      rlp.RawValue
	}
	return p2p.Send(r.w, r.msgcode, resp{r.reqID, bv, r.data})
}


func (r *reply) size() uint32 {
	return uint32(len(r.data))
}


func (p *clientPeer) replyBlockHeaders(reqID uint64, headers []*types.Header) *reply {
	data, _ := rlp.EncodeToBytes(headers)
	return &reply{p.rw, BlockHeadersMsg, reqID, data}
}



func (p *clientPeer) replyBlockBodiesRLP(reqID uint64, bodies []rlp.RawValue) *reply {
	data, _ := rlp.EncodeToBytes(bodies)
	return &reply{p.rw, BlockBodiesMsg, reqID, data}
}



func (p *clientPeer) replyCode(reqID uint64, codes [][]byte) *reply {
	data, _ := rlp.EncodeToBytes(codes)
	return &reply{p.rw, CodeMsg, reqID, data}
}



func (p *clientPeer) replyReceiptsRLP(reqID uint64, receipts []rlp.RawValue) *reply {
	data, _ := rlp.EncodeToBytes(receipts)
	return &reply{p.rw, ReceiptsMsg, reqID, data}
}


func (p *clientPeer) replyProofsV2(reqID uint64, proofs light.NodeList) *reply {
	data, _ := rlp.EncodeToBytes(proofs)
	return &reply{p.rw, ProofsV2Msg, reqID, data}
}


func (p *clientPeer) replyHelperTrieProofs(reqID uint64, resp HelperTrieResps) *reply {
	data, _ := rlp.EncodeToBytes(resp)
	return &reply{p.rw, HelperTrieProofsMsg, reqID, data}
}


func (p *clientPeer) replyTxStatus(reqID uint64, stats []light.TxStatus) *reply {
	data, _ := rlp.EncodeToBytes(stats)
	return &reply{p.rw, TxStatusMsg, reqID, data}
}



func (p *clientPeer) sendAnnounce(request announceData) error {
	return p2p.Send(p.rw, AnnounceMsg, request)
}


func (p *clientPeer) allowInactive() bool {
	return false
}



func (p *clientPeer) updateCapacity(cap uint64) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if cap != p.fcParams.MinRecharge {
		p.fcParams = flowcontrol.ServerParams{MinRecharge: cap, BufLimit: cap * bufLimitRatio}
		p.fcClient.UpdateParams(p.fcParams)
		var kvList keyValueList
		kvList = kvList.add("flowControl/MRR", cap)
		kvList = kvList.add("flowControl/BL", cap*bufLimitRatio)
		p.queueSend(func() { p.sendAnnounce(announceData{Update: kvList}) })
	}
}






func (p *clientPeer) freezeClient() {
	if p.version < lpv3 {
		
		
		atomic.StoreUint32(&p.frozen, 1)
		p.Peer.Disconnect(p2p.DiscUselessPeer)
		return
	}
	if atomic.SwapUint32(&p.frozen, 1) == 0 {
		go func() {
			p.sendStop()
			time.Sleep(freezeTimeBase + time.Duration(rand.Int63n(int64(freezeTimeRandom))))
			for {
				bufValue, bufLimit := p.fcClient.BufferStatus()
				if bufLimit == 0 {
					return
				}
				if bufValue <= bufLimit/8 {
					time.Sleep(freezeCheckPeriod)
				} else {
					atomic.StoreUint32(&p.frozen, 0)
					p.sendResume(bufValue)
					break
				}
			}
		}()
	}
}



func (p *clientPeer) Handshake(td *big.Int, head common.Hash, headNum uint64, genesis common.Hash, server *LesServer) error {
	return p.handshake(td, head, headNum, genesis, func(lists *keyValueList) {
		
		if !server.config.UltraLightOnlyAnnounce {
			*lists = (*lists).add("serveHeaders", nil)
			*lists = (*lists).add("serveChainSince", uint64(0))
			*lists = (*lists).add("serveStateSince", uint64(0))

			
			
			stateRecent := uint64(core.TriesInMemory - 4)
			if server.archiveMode {
				stateRecent = 0
			}
			*lists = (*lists).add("serveRecentState", stateRecent)
			*lists = (*lists).add("txRelay", nil)
		}
		*lists = (*lists).add("flowControl/BL", server.defParams.BufLimit)
		*lists = (*lists).add("flowControl/MRR", server.defParams.MinRecharge)

		var costList RequestCostList
		if server.costTracker.testCostList != nil {
			costList = server.costTracker.testCostList
		} else {
			costList = server.costTracker.makeCostList(server.costTracker.globalFactor())
		}
		*lists = (*lists).add("flowControl/MRC", costList)
		p.fcCosts = costList.decode(ProtocolLengths[uint(p.version)])
		p.fcParams = server.defParams

		
		
		if server.oracle != nil && server.oracle.IsRunning() {
			cp, height := server.oracle.StableCheckpoint()
			if cp != nil {
				*lists = (*lists).add("checkpoint/value", cp)
				*lists = (*lists).add("checkpoint/registerHeight", height)
			}
		}
	}, func(recv keyValueMap) error {
		p.server = recv.get("flowControl/MRR", nil) == nil
		if p.server {
			p.announceType = announceTypeNone 
		} else {
			if recv.get("announceType", &p.announceType) != nil {
				
				p.announceType = announceTypeSimple
			}
			p.fcClient = flowcontrol.NewClientNode(server.fcManager, p.fcParams)
		}
		return nil
	})
}

func (p *clientPeer) bumpInvalid() {
	p.invalidLock.Lock()
	p.invalidCount.Add(1, mclock.Now())
	p.invalidLock.Unlock()
}

func (p *clientPeer) getInvalid() uint64 {
	p.invalidLock.RLock()
	defer p.invalidLock.RUnlock()
	return p.invalidCount.Value(mclock.Now())
}



type serverPeerSubscriber interface {
	registerPeer(*serverPeer)
	unregisterPeer(*serverPeer)
}



type clientPeerSubscriber interface {
	registerPeer(*clientPeer)
	unregisterPeer(*clientPeer)
}



type clientPeerSet struct {
	peers map[string]*clientPeer
	
	
	
	subscribers []clientPeerSubscriber
	closed      bool
	lock        sync.RWMutex
}


func newClientPeerSet() *clientPeerSet {
	return &clientPeerSet{peers: make(map[string]*clientPeer)}
}



func (ps *clientPeerSet) subscribe(sub clientPeerSubscriber) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	ps.subscribers = append(ps.subscribers, sub)
	for _, p := range ps.peers {
		sub.registerPeer(p)
	}
}


func (ps *clientPeerSet) unSubscribe(sub clientPeerSubscriber) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for i, s := range ps.subscribers {
		if s == sub {
			ps.subscribers = append(ps.subscribers[:i], ps.subscribers[i+1:]...)
			return
		}
	}
}



func (ps *clientPeerSet) register(peer *clientPeer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, exist := ps.peers[peer.id]; exist {
		return errAlreadyRegistered
	}
	ps.peers[peer.id] = peer
	for _, sub := range ps.subscribers {
		sub.registerPeer(peer)
	}
	return nil
}




func (ps *clientPeerSet) unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	p, ok := ps.peers[id]
	if !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)
	for _, sub := range ps.subscribers {
		sub.unregisterPeer(p)
	}
	p.Peer.Disconnect(p2p.DiscRequested)
	return nil
}


func (ps *clientPeerSet) ids() []string {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var ids []string
	for id := range ps.peers {
		ids = append(ids, id)
	}
	return ids
}


func (ps *clientPeerSet) peer(id string) *clientPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}


func (ps *clientPeerSet) len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}


func (ps *clientPeerSet) allPeers() []*clientPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*clientPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	return list
}



func (ps *clientPeerSet) close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}



type serverPeerSet struct {
	peers map[string]*serverPeer
	
	
	
	subscribers []serverPeerSubscriber
	closed      bool
	lock        sync.RWMutex
}


func newServerPeerSet() *serverPeerSet {
	return &serverPeerSet{peers: make(map[string]*serverPeer)}
}



func (ps *serverPeerSet) subscribe(sub serverPeerSubscriber) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	ps.subscribers = append(ps.subscribers, sub)
	for _, p := range ps.peers {
		sub.registerPeer(p)
	}
}


func (ps *serverPeerSet) unSubscribe(sub serverPeerSubscriber) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for i, s := range ps.subscribers {
		if s == sub {
			ps.subscribers = append(ps.subscribers[:i], ps.subscribers[i+1:]...)
			return
		}
	}
}



func (ps *serverPeerSet) register(peer *serverPeer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, exist := ps.peers[peer.id]; exist {
		return errAlreadyRegistered
	}
	ps.peers[peer.id] = peer
	for _, sub := range ps.subscribers {
		sub.registerPeer(peer)
	}
	return nil
}




func (ps *serverPeerSet) unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	p, ok := ps.peers[id]
	if !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)
	for _, sub := range ps.subscribers {
		sub.unregisterPeer(p)
	}
	p.Peer.Disconnect(p2p.DiscRequested)
	return nil
}


func (ps *serverPeerSet) ids() []string {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var ids []string
	for id := range ps.peers {
		ids = append(ids, id)
	}
	return ids
}


func (ps *serverPeerSet) peer(id string) *serverPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}


func (ps *serverPeerSet) len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}




func (ps *serverPeerSet) bestPeer() *serverPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *serverPeer
		bestTd   *big.Int
	)
	for _, p := range ps.peers {
		if td := p.Td(); bestTd == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, td
		}
	}
	return bestPeer
}


func (ps *serverPeerSet) allPeers() []*serverPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*serverPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	return list
}



func (ps *serverPeerSet) close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}






type serverSet struct {
	lock   sync.Mutex
	set    map[string]*clientPeer
	closed bool
}

func newServerSet() *serverSet {
	return &serverSet{set: make(map[string]*clientPeer)}
}

func (s *serverSet) register(peer *clientPeer) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return errClosed
	}
	if _, exist := s.set[peer.id]; exist {
		return errAlreadyRegistered
	}
	s.set[peer.id] = peer
	return nil
}

func (s *serverSet) close() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, p := range s.set {
		p.Disconnect(p2p.DiscQuitting)
	}
	s.closed = true
}
