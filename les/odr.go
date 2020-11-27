















package les

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
)


type LesOdr struct {
	db                                         ethdb.Database
	indexerConfig                              *light.IndexerConfig
	chtIndexer, bloomTrieIndexer, bloomIndexer *core.ChainIndexer
	retriever                                  *retrieveManager
	stop                                       chan struct{}
}

func NewLesOdr(db ethdb.Database, config *light.IndexerConfig, retriever *retrieveManager) *LesOdr {
	return &LesOdr{
		db:            db,
		indexerConfig: config,
		retriever:     retriever,
		stop:          make(chan struct{}),
	}
}


func (odr *LesOdr) Stop() {
	close(odr.stop)
}


func (odr *LesOdr) Database() ethdb.Database {
	return odr.db
}


func (odr *LesOdr) SetIndexers(chtIndexer, bloomTrieIndexer, bloomIndexer *core.ChainIndexer) {
	odr.chtIndexer = chtIndexer
	odr.bloomTrieIndexer = bloomTrieIndexer
	odr.bloomIndexer = bloomIndexer
}


func (odr *LesOdr) ChtIndexer() *core.ChainIndexer {
	return odr.chtIndexer
}


func (odr *LesOdr) BloomTrieIndexer() *core.ChainIndexer {
	return odr.bloomTrieIndexer
}


func (odr *LesOdr) BloomIndexer() *core.ChainIndexer {
	return odr.bloomIndexer
}


func (odr *LesOdr) IndexerConfig() *light.IndexerConfig {
	return odr.indexerConfig
}

const (
	MsgBlockBodies = iota
	MsgCode
	MsgReceipts
	MsgProofsV2
	MsgHelperTrieProofs
	MsgTxStatus
)


type Msg struct {
	MsgType int
	ReqID   uint64
	Obj     interface{}
}



func (odr *LesOdr) Retrieve(ctx context.Context, req light.OdrRequest) (err error) {
	lreq := LesRequest(req)

	reqID := genReqID()
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			return lreq.GetCost(dp.(*serverPeer))
		},
		canSend: func(dp distPeer) bool {
			p := dp.(*serverPeer)
			if !p.onlyAnnounce {
				return lreq.CanSend(p)
			}
			return false
		},
		request: func(dp distPeer) func() {
			p := dp.(*serverPeer)
			cost := lreq.GetCost(p)
			p.fcServer.QueuedRequest(reqID, cost)
			return func() { lreq.Request(reqID, p) }
		},
	}
	sent := mclock.Now()
	if err = odr.retriever.retrieve(ctx, reqID, rq, func(p distPeer, msg *Msg) error { return lreq.Validate(odr.db, msg) }, odr.stop); err == nil {
		
		req.StoreResult(odr.db)
		requestRTT.Update(time.Duration(mclock.Now() - sent))
	} else {
		log.Debug("Failed to retrieve data from network", "err", err)
	}
	return
}
