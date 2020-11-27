















package les

import (
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/fetcher"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

const (
	blockDelayTimeout    = 10 * time.Second       
	gatherSlack          = 100 * time.Millisecond 
	cachedAnnosThreshold = 64                     
)


type announce struct {
	data   *announceData
	trust  bool
	peerid enode.ID
}


type request struct {
	reqid  uint64
	peerid enode.ID
	sendAt time.Time
	hash   common.Hash
}



type response struct {
	reqid   uint64
	headers []*types.Header
	peerid  enode.ID
	remain  chan []*types.Header
}


type fetcherPeer struct {
	latest *announceData 

	
	
	
	announces     map[common.Hash]*announce 
	announcesList []common.Hash             
}



func (fp *fetcherPeer) addAnno(anno *announce) {
	
	
	
	
	hash := anno.data.Hash
	if _, exist := fp.announces[hash]; exist {
		return
	}
	fp.announces[hash] = anno
	fp.announcesList = append(fp.announcesList, hash)

	
	if len(fp.announcesList)-cachedAnnosThreshold > 0 {
		for i := 0; i < len(fp.announcesList)-cachedAnnosThreshold; i++ {
			delete(fp.announces, fp.announcesList[i])
		}
		copy(fp.announcesList, fp.announcesList[len(fp.announcesList)-cachedAnnosThreshold:])
		fp.announcesList = fp.announcesList[:cachedAnnosThreshold]
	}
}



func (fp *fetcherPeer) forwardAnno(td *big.Int) []*announce {
	var (
		cutset  int
		evicted []*announce
	)
	for ; cutset < len(fp.announcesList); cutset++ {
		anno := fp.announces[fp.announcesList[cutset]]
		if anno == nil {
			continue 
		}
		if anno.data.Td.Cmp(td) > 0 {
			break
		}
		evicted = append(evicted, anno)
		delete(fp.announces, anno.data.Hash)
	}
	if cutset > 0 {
		copy(fp.announcesList, fp.announcesList[cutset:])
		fp.announcesList = fp.announcesList[:len(fp.announcesList)-cutset]
	}
	return evicted
}




type lightFetcher struct {
	
	ulc     *ulc
	chaindb ethdb.Database
	reqDist *requestDistributor
	peerset *serverPeerSet        
	chain   *light.LightChain     
	fetcher *fetcher.BlockFetcher 

	
	plock sync.RWMutex
	peers map[enode.ID]*fetcherPeer

	
	announceCh chan *announce
	requestCh  chan *request
	deliverCh  chan *response
	syncDone   chan *types.Header

	closeCh chan struct{}
	wg      sync.WaitGroup

	
	synchronise func(peer *serverPeer)

	
	noAnnounce  bool
	newHeadHook func(*types.Header)
	newAnnounce func(*serverPeer, *announceData)
}


func newLightFetcher(chain *light.LightChain, engine consensus.Engine, peers *serverPeerSet, ulc *ulc, chaindb ethdb.Database, reqDist *requestDistributor, syncFn func(p *serverPeer)) *lightFetcher {
	
	validator := func(header *types.Header) error {
		
		return engine.VerifyHeader(chain, header, ulc == nil)
	}
	heighter := func() uint64 { return chain.CurrentHeader().Number.Uint64() }
	dropper := func(id string) { peers.unregister(id) }
	inserter := func(headers []*types.Header) (int, error) {
		
		checkFreq := 1
		if ulc != nil {
			checkFreq = 0
		}
		return chain.InsertHeaderChain(headers, checkFreq)
	}
	f := &lightFetcher{
		ulc:         ulc,
		peerset:     peers,
		chaindb:     chaindb,
		chain:       chain,
		reqDist:     reqDist,
		fetcher:     fetcher.NewBlockFetcher(true, chain.GetHeaderByHash, nil, validator, nil, heighter, inserter, nil, dropper),
		peers:       make(map[enode.ID]*fetcherPeer),
		synchronise: syncFn,
		announceCh:  make(chan *announce),
		requestCh:   make(chan *request),
		deliverCh:   make(chan *response),
		syncDone:    make(chan *types.Header),
		closeCh:     make(chan struct{}),
	}
	peers.subscribe(f)
	return f
}

func (f *lightFetcher) start() {
	f.wg.Add(1)
	f.fetcher.Start()
	go f.mainloop()
}

func (f *lightFetcher) stop() {
	close(f.closeCh)
	f.fetcher.Stop()
	f.wg.Wait()
}


func (f *lightFetcher) registerPeer(p *serverPeer) {
	f.plock.Lock()
	defer f.plock.Unlock()

	f.peers[p.ID()] = &fetcherPeer{announces: make(map[common.Hash]*announce)}
}


func (f *lightFetcher) unregisterPeer(p *serverPeer) {
	f.plock.Lock()
	defer f.plock.Unlock()

	delete(f.peers, p.ID())
}


func (f *lightFetcher) peer(id enode.ID) *fetcherPeer {
	f.plock.RLock()
	defer f.plock.RUnlock()

	return f.peers[id]
}



func (f *lightFetcher) forEachPeer(check func(id enode.ID, p *fetcherPeer) bool) {
	f.plock.RLock()
	defer f.plock.RUnlock()

	for id, peer := range f.peers {
		if !check(id, peer) {
			return
		}
	}
}














func (f *lightFetcher) mainloop() {
	defer f.wg.Done()

	var (
		syncInterval = uint64(1) 
		syncing      bool        

		ulc          = f.ulc != nil
		headCh       = make(chan core.ChainHeadEvent, 100)
		fetching     = make(map[uint64]*request)
		requestTimer = time.NewTimer(0)

		
		localHead = f.chain.CurrentHeader()
		localTd   = f.chain.GetTd(localHead.Hash(), localHead.Number.Uint64())
	)
	sub := f.chain.SubscribeChainHeadEvent(headCh)
	defer sub.Unsubscribe()

	
	reset := func(header *types.Header) {
		localHead = header
		localTd = f.chain.GetTd(header.Hash(), header.Number.Uint64())
	}
	
	
	
	trustedHeader := func(hash common.Hash, number uint64) (bool, []enode.ID) {
		var (
			agreed  []enode.ID
			trusted bool
		)
		f.forEachPeer(func(id enode.ID, p *fetcherPeer) bool {
			if anno := p.announces[hash]; anno != nil && anno.trust && anno.data.Number == number {
				agreed = append(agreed, id)
				if 100*len(agreed)/len(f.ulc.keys) >= f.ulc.fraction {
					trusted = true
					return false 
				}
			}
			return true
		})
		return trusted, agreed
	}
	for {
		select {
		case anno := <-f.announceCh:
			peerid, data := anno.peerid, anno.data
			log.Debug("Received new announce", "peer", peerid, "number", data.Number, "hash", data.Hash, "reorg", data.ReorgDepth)

			peer := f.peer(peerid)
			if peer == nil {
				log.Debug("Receive announce from unknown peer", "peer", peerid)
				continue
			}
			
			
			if peer.latest != nil && data.Td.Cmp(peer.latest.Td) <= 0 {
				f.peerset.unregister(peerid.String())
				log.Debug("Non-monotonic td", "peer", peerid, "current", data.Td, "previous", peer.latest.Td)
				continue
			}
			peer.latest = data

			
			if localTd != nil && data.Td.Cmp(localTd) <= 0 {
				continue
			}
			peer.addAnno(anno)

			
			if !ulc && !syncing {
				
				
				
				
				
				if data.Number > localHead.Number.Uint64()+syncInterval || data.ReorgDepth > 0 {
					syncing = true
					go f.startSync(peerid)
					log.Debug("Trigger light sync", "peer", peerid, "local", localHead.Number, "localhash", localHead.Hash(), "remote", data.Number, "remotehash", data.Hash)
					continue
				}
				f.fetcher.Notify(peerid.String(), data.Hash, data.Number, time.Now(), f.requestHeaderByHash(peerid), nil)
				log.Debug("Trigger header retrieval", "peer", peerid, "number", data.Number, "hash", data.Hash)
			}
			
			if ulc && anno.trust {
				
				
				trusted, agreed := trustedHeader(data.Hash, data.Number)
				if trusted && !syncing {
					if data.Number > localHead.Number.Uint64()+syncInterval || data.ReorgDepth > 0 {
						syncing = true
						go f.startSync(peerid)
						log.Debug("Trigger trusted light sync", "local", localHead.Number, "localhash", localHead.Hash(), "remote", data.Number, "remotehash", data.Hash)
						continue
					}
					p := agreed[rand.Intn(len(agreed))]
					f.fetcher.Notify(p.String(), data.Hash, data.Number, time.Now(), f.requestHeaderByHash(p), nil)
					log.Debug("Trigger trusted header retrieval", "number", data.Number, "hash", data.Hash)
				}
			}

		case req := <-f.requestCh:
			fetching[req.reqid] = req 
			if len(fetching) == 1 {
				f.rescheduleTimer(fetching, requestTimer)
			}

		case <-requestTimer.C:
			for reqid, request := range fetching {
				if time.Since(request.sendAt) > blockDelayTimeout-gatherSlack {
					delete(fetching, reqid)
					f.peerset.unregister(request.peerid.String())
					log.Debug("Request timeout", "peer", request.peerid, "reqid", reqid)
				}
			}
			f.rescheduleTimer(fetching, requestTimer)

		case resp := <-f.deliverCh:
			if req := fetching[resp.reqid]; req != nil {
				delete(fetching, resp.reqid)
				f.rescheduleTimer(fetching, requestTimer)

				
				
				
				
				if len(resp.headers) != 1 {
					f.peerset.unregister(req.peerid.String())
					log.Debug("Deliver more than requested", "peer", req.peerid, "reqid", req.reqid)
					continue
				}
				if resp.headers[0].Hash() != req.hash {
					f.peerset.unregister(req.peerid.String())
					log.Debug("Deliver invalid header", "peer", req.peerid, "reqid", req.reqid)
					continue
				}
				resp.remain <- f.fetcher.FilterHeaders(resp.peerid.String(), resp.headers, time.Now())
			} else {
				
				resp.remain <- resp.headers
			}

		case ev := <-headCh:
			
			if syncing {
				continue
			}
			reset(ev.Block.Header())

			
			var droplist []enode.ID
			f.forEachPeer(func(id enode.ID, p *fetcherPeer) bool {
				removed := p.forwardAnno(localTd)
				for _, anno := range removed {
					if header := f.chain.GetHeaderByHash(anno.data.Hash); header != nil {
						if header.Number.Uint64() != anno.data.Number {
							droplist = append(droplist, id)
							break
						}
						
						td := f.chain.GetTd(anno.data.Hash, anno.data.Number)
						if td != nil && td.Cmp(anno.data.Td) != 0 {
							droplist = append(droplist, id)
							break
						}
					}
				}
				return true
			})
			for _, id := range droplist {
				f.peerset.unregister(id.String())
				log.Debug("Kicked out peer for invalid announcement")
			}
			if f.newHeadHook != nil {
				f.newHeadHook(localHead)
			}

		case origin := <-f.syncDone:
			syncing = false 

			
			if ulc {
				head := f.chain.CurrentHeader()
				ancestor := rawdb.FindCommonAncestor(f.chaindb, origin, head)
				var untrusted []common.Hash
				for head.Number.Cmp(ancestor.Number) > 0 {
					hash, number := head.Hash(), head.Number.Uint64()
					if trusted, _ := trustedHeader(hash, number); trusted {
						break
					}
					untrusted = append(untrusted, hash)
					head = f.chain.GetHeader(head.ParentHash, number-1)
				}
				if len(untrusted) > 0 {
					for i, j := 0, len(untrusted)-1; i < j; i, j = i+1, j-1 {
						untrusted[i], untrusted[j] = untrusted[j], untrusted[i]
					}
					f.chain.Rollback(untrusted)
				}
			}
			
			reset(f.chain.CurrentHeader())
			if f.newHeadHook != nil {
				f.newHeadHook(localHead)
			}
			log.Debug("light sync finished", "number", localHead.Number, "hash", localHead.Hash())

		case <-f.closeCh:
			return
		}
	}
}


func (f *lightFetcher) announce(p *serverPeer, head *announceData) {
	if f.newAnnounce != nil {
		f.newAnnounce(p, head)
	}
	if f.noAnnounce {
		return
	}
	select {
	case f.announceCh <- &announce{peerid: p.ID(), trust: p.trusted, data: head}:
	case <-f.closeCh:
		return
	}
}


func (f *lightFetcher) trackRequest(peerid enode.ID, reqid uint64, hash common.Hash) {
	select {
	case f.requestCh <- &request{reqid: reqid, peerid: peerid, sendAt: time.Now(), hash: hash}:
	case <-f.closeCh:
	}
}







func (f *lightFetcher) requestHeaderByHash(peerid enode.ID) func(common.Hash) error {
	return func(hash common.Hash) error {
		req := &distReq{
			getCost: func(dp distPeer) uint64 { return dp.(*serverPeer).getRequestCost(GetBlockHeadersMsg, 1) },
			canSend: func(dp distPeer) bool { return dp.(*serverPeer).ID() == peerid },
			request: func(dp distPeer) func() {
				peer, id := dp.(*serverPeer), genReqID()
				cost := peer.getRequestCost(GetBlockHeadersMsg, 1)
				peer.fcServer.QueuedRequest(id, cost)

				return func() {
					f.trackRequest(peer.ID(), id, hash)
					peer.requestHeadersByHash(id, hash, 1, 0, false)
				}
			},
		}
		f.reqDist.queue(req)
		return nil
	}
}


func (f *lightFetcher) startSync(id enode.ID) {
	defer func(header *types.Header) {
		f.syncDone <- header
	}(f.chain.CurrentHeader())

	peer := f.peerset.peer(id.String())
	if peer == nil || peer.onlyAnnounce {
		return
	}
	f.synchronise(peer)
}


func (f *lightFetcher) deliverHeaders(peer *serverPeer, reqid uint64, headers []*types.Header) []*types.Header {
	remain := make(chan []*types.Header, 1)
	select {
	case f.deliverCh <- &response{reqid: reqid, headers: headers, peerid: peer.ID(), remain: remain}:
	case <-f.closeCh:
		return nil
	}
	return <-remain
}


func (f *lightFetcher) rescheduleTimer(requests map[uint64]*request, timer *time.Timer) {
	
	if len(requests) == 0 {
		timer.Stop()
		return
	}
	
	earliest := time.Now()
	for _, req := range requests {
		if earliest.After(req.sendAt) {
			earliest = req.sendAt
		}
	}
	timer.Reset(blockDelayTimeout - time.Since(earliest))
}
