















package eth

import (
	"math/big"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

const (
	forceSyncCycle      = 10 * time.Second 
	defaultMinSyncPeers = 5                

	
	
	txsyncPackSize = 100 * 1024
)

type txsync struct {
	p   *peer
	txs []*types.Transaction
}


func (pm *ProtocolManager) syncTransactions(p *peer) {
	
	
	
	
	
	
	var txs types.Transactions
	pending, _ := pm.txpool.Pending()
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	if len(txs) == 0 {
		return
	}
	
	
	
	if p.version >= eth65 {
		hashes := make([]common.Hash, len(txs))
		for i, tx := range txs {
			hashes[i] = tx.Hash()
		}
		p.AsyncSendPooledTransactionHashes(hashes)
		return
	}
	
	select {
	case pm.txsyncCh <- &txsync{p: p, txs: txs}:
	case <-pm.quitSync:
	}
}





func (pm *ProtocolManager) txsyncLoop64() {
	defer pm.wg.Done()

	var (
		pending = make(map[enode.ID]*txsync)
		sending = false               
		pack    = new(txsync)         
		done    = make(chan error, 1) 
	)

	
	send := func(s *txsync) {
		if s.p.version >= eth65 {
			panic("initial transaction syncer running on eth/65+")
		}
		
		size := common.StorageSize(0)
		pack.p = s.p
		pack.txs = pack.txs[:0]
		for i := 0; i < len(s.txs) && size < txsyncPackSize; i++ {
			pack.txs = append(pack.txs, s.txs[i])
			size += s.txs[i].Size()
		}
		
		s.txs = s.txs[:copy(s.txs, s.txs[len(pack.txs):])]
		if len(s.txs) == 0 {
			delete(pending, s.p.ID())
		}
		
		s.p.Log().Trace("Sending batch of transactions", "count", len(pack.txs), "bytes", size)
		sending = true
		go func() { done <- pack.p.SendTransactions64(pack.txs) }()
	}

	
	pick := func() *txsync {
		if len(pending) == 0 {
			return nil
		}
		n := rand.Intn(len(pending)) + 1
		for _, s := range pending {
			if n--; n == 0 {
				return s
			}
		}
		return nil
	}

	for {
		select {
		case s := <-pm.txsyncCh:
			pending[s.p.ID()] = s
			if !sending {
				send(s)
			}
		case err := <-done:
			sending = false
			
			if err != nil {
				pack.p.Log().Debug("Transaction send failed", "err", err)
				delete(pending, pack.p.ID())
			}
			
			if s := pick(); s != nil {
				send(s)
			}
		case <-pm.quitSync:
			return
		}
	}
}


type chainSyncer struct {
	pm          *ProtocolManager
	force       *time.Timer
	forced      bool 
	peerEventCh chan struct{}
	doneCh      chan error 
}


type chainSyncOp struct {
	mode downloader.SyncMode
	peer *peer
	td   *big.Int
	head common.Hash
}


func newChainSyncer(pm *ProtocolManager) *chainSyncer {
	return &chainSyncer{
		pm:          pm,
		peerEventCh: make(chan struct{}),
	}
}




func (cs *chainSyncer) handlePeerEvent(p *peer) bool {
	select {
	case cs.peerEventCh <- struct{}{}:
		return true
	case <-cs.pm.quitSync:
		return false
	}
}


func (cs *chainSyncer) loop() {
	defer cs.pm.wg.Done()

	cs.pm.blockFetcher.Start()
	cs.pm.txFetcher.Start()
	defer cs.pm.blockFetcher.Stop()
	defer cs.pm.txFetcher.Stop()

	
	
	cs.force = time.NewTimer(forceSyncCycle)
	defer cs.force.Stop()

	for {
		if op := cs.nextSyncOp(); op != nil {
			cs.startSync(op)
		}

		select {
		case <-cs.peerEventCh:
			
		case <-cs.doneCh:
			cs.doneCh = nil
			cs.force.Reset(forceSyncCycle)
			cs.forced = false
		case <-cs.force.C:
			cs.forced = true

		case <-cs.pm.quitSync:
			
			
			
			cs.pm.blockchain.StopInsert()
			cs.pm.downloader.Terminate()
			if cs.doneCh != nil {
				
				<-cs.doneCh
			}
			return
		}
	}
}


func (cs *chainSyncer) nextSyncOp() *chainSyncOp {
	if cs.doneCh != nil {
		return nil 
	}

	
	minPeers := defaultMinSyncPeers
	if cs.forced {
		minPeers = 1
	} else if minPeers > cs.pm.maxPeers {
		minPeers = cs.pm.maxPeers
	}
	if cs.pm.peers.Len() < minPeers {
		return nil
	}

	
	peer := cs.pm.peers.BestPeer()
	if peer == nil {
		return nil
	}
	mode, ourTD := cs.modeAndLocalHead()
	op := peerToSyncOp(mode, peer)
	if op.td.Cmp(ourTD) <= 0 {
		return nil 
	}
	return op
}

func peerToSyncOp(mode downloader.SyncMode, p *peer) *chainSyncOp {
	peerHead, peerTD := p.Head()
	return &chainSyncOp{mode: mode, peer: p, td: peerTD, head: peerHead}
}

func (cs *chainSyncer) modeAndLocalHead() (downloader.SyncMode, *big.Int) {
	
	if atomic.LoadUint32(&cs.pm.fastSync) == 1 {
		block := cs.pm.blockchain.CurrentFastBlock()
		td := cs.pm.blockchain.GetTdByHash(block.Hash())
		return downloader.FastSync, td
	}
	
	
	if pivot := rawdb.ReadLastPivotNumber(cs.pm.chaindb); pivot != nil {
		if head := cs.pm.blockchain.CurrentBlock(); head.NumberU64() < *pivot {
			block := cs.pm.blockchain.CurrentFastBlock()
			td := cs.pm.blockchain.GetTdByHash(block.Hash())
			return downloader.FastSync, td
		}
	}
	
	head := cs.pm.blockchain.CurrentHeader()
	td := cs.pm.blockchain.GetTd(head.Hash(), head.Number.Uint64())
	return downloader.FullSync, td
}


func (cs *chainSyncer) startSync(op *chainSyncOp) {
	cs.doneCh = make(chan error, 1)
	go func() { cs.doneCh <- cs.pm.doSync(op) }()
}


func (pm *ProtocolManager) doSync(op *chainSyncOp) error {
	if op.mode == downloader.FastSync {
		
		
		
		
		
		
		
		
		
		limit := pm.blockchain.TxLookupLimit()
		if stored := rawdb.ReadFastTxLookupLimit(pm.chaindb); stored == nil {
			rawdb.WriteFastTxLookupLimit(pm.chaindb, limit)
		} else if *stored != limit {
			pm.blockchain.SetTxLookupLimit(*stored)
			log.Warn("Update txLookup limit", "provided", limit, "updated", *stored)
		}
	}
	
	err := pm.downloader.Synchronise(op.peer.id, op.head, op.td, op.mode)
	if err != nil {
		return err
	}
	if atomic.LoadUint32(&pm.fastSync) == 1 {
		log.Info("Fast sync complete, auto disabling")
		atomic.StoreUint32(&pm.fastSync, 0)
	}

	
	
	head := pm.blockchain.CurrentBlock()
	if head.NumberU64() >= pm.checkpointNumber {
		
		
		if head.Time() >= uint64(time.Now().AddDate(0, -1, 0).Unix()) {
			atomic.StoreUint32(&pm.acceptTxs, 1)
		}
	}

	if head.NumberU64() > 0 {
		
		
		
		
		
		
		pm.BroadcastBlock(head, false)
	}

	return nil
}
