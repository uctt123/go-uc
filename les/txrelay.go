















package les

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

type ltrInfo struct {
	tx     *types.Transaction
	sentTo map[*serverPeer]struct{}
}

type lesTxRelay struct {
	txSent       map[common.Hash]*ltrInfo
	txPending    map[common.Hash]struct{}
	peerList     []*serverPeer
	peerStartPos int
	lock         sync.Mutex
	stop         chan struct{}

	retriever *retrieveManager
}

func newLesTxRelay(ps *serverPeerSet, retriever *retrieveManager) *lesTxRelay {
	r := &lesTxRelay{
		txSent:    make(map[common.Hash]*ltrInfo),
		txPending: make(map[common.Hash]struct{}),
		retriever: retriever,
		stop:      make(chan struct{}),
	}
	ps.subscribe(r)
	return r
}

func (ltrx *lesTxRelay) Stop() {
	close(ltrx.stop)
}

func (ltrx *lesTxRelay) registerPeer(p *serverPeer) {
	ltrx.lock.Lock()
	defer ltrx.lock.Unlock()

	
	if p.onlyAnnounce {
		return
	}
	ltrx.peerList = append(ltrx.peerList, p)
}

func (ltrx *lesTxRelay) unregisterPeer(p *serverPeer) {
	ltrx.lock.Lock()
	defer ltrx.lock.Unlock()

	for i, peer := range ltrx.peerList {
		if peer == p {
			
			ltrx.peerList = append(ltrx.peerList[:i], ltrx.peerList[i+1:]...)
			return
		}
	}
}



func (ltrx *lesTxRelay) send(txs types.Transactions, count int) {
	sendTo := make(map[*serverPeer]types.Transactions)

	ltrx.peerStartPos++ 
	if ltrx.peerStartPos >= len(ltrx.peerList) {
		ltrx.peerStartPos = 0
	}

	for _, tx := range txs {
		hash := tx.Hash()
		ltr, ok := ltrx.txSent[hash]
		if !ok {
			ltr = &ltrInfo{
				tx:     tx,
				sentTo: make(map[*serverPeer]struct{}),
			}
			ltrx.txSent[hash] = ltr
			ltrx.txPending[hash] = struct{}{}
		}

		if len(ltrx.peerList) > 0 {
			cnt := count
			pos := ltrx.peerStartPos
			for {
				peer := ltrx.peerList[pos]
				if _, ok := ltr.sentTo[peer]; !ok {
					sendTo[peer] = append(sendTo[peer], tx)
					ltr.sentTo[peer] = struct{}{}
					cnt--
				}
				if cnt == 0 {
					break 
				}
				pos++
				if pos == len(ltrx.peerList) {
					pos = 0
				}
				if pos == ltrx.peerStartPos {
					break 
				}
			}
		}
	}

	for p, list := range sendTo {
		pp := p
		ll := list
		enc, _ := rlp.EncodeToBytes(ll)

		reqID := genReqID()
		rq := &distReq{
			getCost: func(dp distPeer) uint64 {
				peer := dp.(*serverPeer)
				return peer.getTxRelayCost(len(ll), len(enc))
			},
			canSend: func(dp distPeer) bool {
				return !dp.(*serverPeer).onlyAnnounce && dp.(*serverPeer) == pp
			},
			request: func(dp distPeer) func() {
				peer := dp.(*serverPeer)
				cost := peer.getTxRelayCost(len(ll), len(enc))
				peer.fcServer.QueuedRequest(reqID, cost)
				return func() { peer.sendTxs(reqID, len(ll), enc) }
			},
		}
		go ltrx.retriever.retrieve(context.Background(), reqID, rq, func(p distPeer, msg *Msg) error { return nil }, ltrx.stop)
	}
}

func (ltrx *lesTxRelay) Send(txs types.Transactions) {
	ltrx.lock.Lock()
	defer ltrx.lock.Unlock()

	ltrx.send(txs, 3)
}

func (ltrx *lesTxRelay) NewHead(head common.Hash, mined []common.Hash, rollback []common.Hash) {
	ltrx.lock.Lock()
	defer ltrx.lock.Unlock()

	for _, hash := range mined {
		delete(ltrx.txPending, hash)
	}

	for _, hash := range rollback {
		ltrx.txPending[hash] = struct{}{}
	}

	if len(ltrx.txPending) > 0 {
		txs := make(types.Transactions, len(ltrx.txPending))
		i := 0
		for hash := range ltrx.txPending {
			txs[i] = ltrx.txSent[hash].tx
			i++
		}
		ltrx.send(txs, 1)
	}
}

func (ltrx *lesTxRelay) Discard(hashes []common.Hash) {
	ltrx.lock.Lock()
	defer ltrx.lock.Unlock()

	for _, hash := range hashes {
		delete(ltrx.txSent, hash)
		delete(ltrx.txPending, hash)
	}
}
