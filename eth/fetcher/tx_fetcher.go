















package fetcher

import (
	"bytes"
	"fmt"
	mrand "math/rand"
	"sort"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
)

const (
	
	
	maxTxAnnounces = 4096

	
	
	
	
	
	
	
	
	maxTxRetrievals = 256

	
	
	
	maxTxUnderpricedSetSize = 32768

	
	
	txArriveTimeout = 500 * time.Millisecond

	
	
	txGatherSlack = 100 * time.Millisecond
)

var (
	
	
	txFetchTimeout = 5 * time.Second
)

var (
	txAnnounceInMeter          = metrics.NewRegisteredMeter("eth/fetcher/transaction/announces/in", nil)
	txAnnounceKnownMeter       = metrics.NewRegisteredMeter("eth/fetcher/transaction/announces/known", nil)
	txAnnounceUnderpricedMeter = metrics.NewRegisteredMeter("eth/fetcher/transaction/announces/underpriced", nil)
	txAnnounceDOSMeter         = metrics.NewRegisteredMeter("eth/fetcher/transaction/announces/dos", nil)

	txBroadcastInMeter          = metrics.NewRegisteredMeter("eth/fetcher/transaction/broadcasts/in", nil)
	txBroadcastKnownMeter       = metrics.NewRegisteredMeter("eth/fetcher/transaction/broadcasts/known", nil)
	txBroadcastUnderpricedMeter = metrics.NewRegisteredMeter("eth/fetcher/transaction/broadcasts/underpriced", nil)
	txBroadcastOtherRejectMeter = metrics.NewRegisteredMeter("eth/fetcher/transaction/broadcasts/otherreject", nil)

	txRequestOutMeter     = metrics.NewRegisteredMeter("eth/fetcher/transaction/request/out", nil)
	txRequestFailMeter    = metrics.NewRegisteredMeter("eth/fetcher/transaction/request/fail", nil)
	txRequestDoneMeter    = metrics.NewRegisteredMeter("eth/fetcher/transaction/request/done", nil)
	txRequestTimeoutMeter = metrics.NewRegisteredMeter("eth/fetcher/transaction/request/timeout", nil)

	txReplyInMeter          = metrics.NewRegisteredMeter("eth/fetcher/transaction/replies/in", nil)
	txReplyKnownMeter       = metrics.NewRegisteredMeter("eth/fetcher/transaction/replies/known", nil)
	txReplyUnderpricedMeter = metrics.NewRegisteredMeter("eth/fetcher/transaction/replies/underpriced", nil)
	txReplyOtherRejectMeter = metrics.NewRegisteredMeter("eth/fetcher/transaction/replies/otherreject", nil)

	txFetcherWaitingPeers   = metrics.NewRegisteredGauge("eth/fetcher/transaction/waiting/peers", nil)
	txFetcherWaitingHashes  = metrics.NewRegisteredGauge("eth/fetcher/transaction/waiting/hashes", nil)
	txFetcherQueueingPeers  = metrics.NewRegisteredGauge("eth/fetcher/transaction/queueing/peers", nil)
	txFetcherQueueingHashes = metrics.NewRegisteredGauge("eth/fetcher/transaction/queueing/hashes", nil)
	txFetcherFetchingPeers  = metrics.NewRegisteredGauge("eth/fetcher/transaction/fetching/peers", nil)
	txFetcherFetchingHashes = metrics.NewRegisteredGauge("eth/fetcher/transaction/fetching/hashes", nil)
)



type txAnnounce struct {
	origin string        
	hashes []common.Hash 
}



type txRequest struct {
	hashes []common.Hash            
	stolen map[common.Hash]struct{} 
	time   mclock.AbsTime           
}



type txDelivery struct {
	origin string        
	hashes []common.Hash 
	direct bool          
}


type txDrop struct {
	peer string
}


















type TxFetcher struct {
	notify  chan *txAnnounce
	cleanup chan *txDelivery
	drop    chan *txDrop
	quit    chan struct{}

	underpriced mapset.Set 

	
	
	waitlist  map[common.Hash]map[string]struct{} 
	waittime  map[common.Hash]mclock.AbsTime      
	waitslots map[string]map[common.Hash]struct{} 

	
	
	announces map[string]map[common.Hash]struct{} 
	announced map[common.Hash]map[string]struct{} 

	
	
	
	fetching   map[common.Hash]string              
	requests   map[string]*txRequest               
	alternates map[common.Hash]map[string]struct{} 

	
	hasTx    func(common.Hash) bool             
	addTxs   func([]*types.Transaction) []error 
	fetchTxs func(string, []common.Hash) error  

	step  chan struct{} 
	clock mclock.Clock  
	rand  *mrand.Rand   
}



func NewTxFetcher(hasTx func(common.Hash) bool, addTxs func([]*types.Transaction) []error, fetchTxs func(string, []common.Hash) error) *TxFetcher {
	return NewTxFetcherForTests(hasTx, addTxs, fetchTxs, mclock.System{}, nil)
}



func NewTxFetcherForTests(
	hasTx func(common.Hash) bool, addTxs func([]*types.Transaction) []error, fetchTxs func(string, []common.Hash) error,
	clock mclock.Clock, rand *mrand.Rand) *TxFetcher {
	return &TxFetcher{
		notify:      make(chan *txAnnounce),
		cleanup:     make(chan *txDelivery),
		drop:        make(chan *txDrop),
		quit:        make(chan struct{}),
		waitlist:    make(map[common.Hash]map[string]struct{}),
		waittime:    make(map[common.Hash]mclock.AbsTime),
		waitslots:   make(map[string]map[common.Hash]struct{}),
		announces:   make(map[string]map[common.Hash]struct{}),
		announced:   make(map[common.Hash]map[string]struct{}),
		fetching:    make(map[common.Hash]string),
		requests:    make(map[string]*txRequest),
		alternates:  make(map[common.Hash]map[string]struct{}),
		underpriced: mapset.NewSet(),
		hasTx:       hasTx,
		addTxs:      addTxs,
		fetchTxs:    fetchTxs,
		clock:       clock,
		rand:        rand,
	}
}



func (f *TxFetcher) Notify(peer string, hashes []common.Hash) error {
	
	txAnnounceInMeter.Mark(int64(len(hashes)))

	
	
	
	
	
	var (
		unknowns               = make([]common.Hash, 0, len(hashes))
		duplicate, underpriced int64
	)
	for _, hash := range hashes {
		switch {
		case f.hasTx(hash):
			duplicate++

		case f.underpriced.Contains(hash):
			underpriced++

		default:
			unknowns = append(unknowns, hash)
		}
	}
	txAnnounceKnownMeter.Mark(duplicate)
	txAnnounceUnderpricedMeter.Mark(underpriced)

	
	if len(unknowns) == 0 {
		return nil
	}
	announce := &txAnnounce{
		origin: peer,
		hashes: unknowns,
	}
	select {
	case f.notify <- announce:
		return nil
	case <-f.quit:
		return errTerminated
	}
}





func (f *TxFetcher) Enqueue(peer string, txs []*types.Transaction, direct bool) error {
	
	if direct {
		txReplyInMeter.Mark(int64(len(txs)))
	} else {
		txBroadcastInMeter.Mark(int64(len(txs)))
	}
	
	
	var (
		added       = make([]common.Hash, 0, len(txs))
		duplicate   int64
		underpriced int64
		otherreject int64
	)
	errs := f.addTxs(txs)
	for i, err := range errs {
		if err != nil {
			
			
			
			if err == core.ErrUnderpriced || err == core.ErrReplaceUnderpriced {
				for f.underpriced.Cardinality() >= maxTxUnderpricedSetSize {
					f.underpriced.Pop()
				}
				f.underpriced.Add(txs[i].Hash())
			}
			
			switch err {
			case nil: 

			case core.ErrAlreadyKnown:
				duplicate++

			case core.ErrUnderpriced, core.ErrReplaceUnderpriced:
				underpriced++

			default:
				otherreject++
			}
		}
		added = append(added, txs[i].Hash())
	}
	if direct {
		txReplyKnownMeter.Mark(duplicate)
		txReplyUnderpricedMeter.Mark(underpriced)
		txReplyOtherRejectMeter.Mark(otherreject)
	} else {
		txBroadcastKnownMeter.Mark(duplicate)
		txBroadcastUnderpricedMeter.Mark(underpriced)
		txBroadcastOtherRejectMeter.Mark(otherreject)
	}
	select {
	case f.cleanup <- &txDelivery{origin: peer, hashes: added, direct: direct}:
		return nil
	case <-f.quit:
		return errTerminated
	}
}



func (f *TxFetcher) Drop(peer string) error {
	select {
	case f.drop <- &txDrop{peer: peer}:
		return nil
	case <-f.quit:
		return errTerminated
	}
}



func (f *TxFetcher) Start() {
	go f.loop()
}



func (f *TxFetcher) Stop() {
	close(f.quit)
}

func (f *TxFetcher) loop() {
	var (
		waitTimer    = new(mclock.Timer)
		timeoutTimer = new(mclock.Timer)

		waitTrigger    = make(chan struct{}, 1)
		timeoutTrigger = make(chan struct{}, 1)
	)
	for {
		select {
		case ann := <-f.notify:
			
			
			
			
			used := len(f.waitslots[ann.origin]) + len(f.announces[ann.origin])
			if used >= maxTxAnnounces {
				
				
				
				
				txAnnounceDOSMeter.Mark(int64(len(ann.hashes)))
				break
			}
			want := used + len(ann.hashes)
			if want > maxTxAnnounces {
				txAnnounceDOSMeter.Mark(int64(want - maxTxAnnounces))
				ann.hashes = ann.hashes[:want-maxTxAnnounces]
			}
			
			idleWait := len(f.waittime) == 0
			_, oldPeer := f.announces[ann.origin]

			for _, hash := range ann.hashes {
				
				
				
				if f.alternates[hash] != nil {
					f.alternates[hash][ann.origin] = struct{}{}

					
					if announces := f.announces[ann.origin]; announces != nil {
						announces[hash] = struct{}{}
					} else {
						f.announces[ann.origin] = map[common.Hash]struct{}{hash: {}}
					}
					continue
				}
				
				
				if f.announced[hash] != nil {
					f.announced[hash][ann.origin] = struct{}{}

					
					if announces := f.announces[ann.origin]; announces != nil {
						announces[hash] = struct{}{}
					} else {
						f.announces[ann.origin] = map[common.Hash]struct{}{hash: {}}
					}
					continue
				}
				
				
				
				if f.waitlist[hash] != nil {
					f.waitlist[hash][ann.origin] = struct{}{}

					if waitslots := f.waitslots[ann.origin]; waitslots != nil {
						waitslots[hash] = struct{}{}
					} else {
						f.waitslots[ann.origin] = map[common.Hash]struct{}{hash: {}}
					}
					continue
				}
				
				f.waitlist[hash] = map[string]struct{}{ann.origin: {}}
				f.waittime[hash] = f.clock.Now()

				if waitslots := f.waitslots[ann.origin]; waitslots != nil {
					waitslots[hash] = struct{}{}
				} else {
					f.waitslots[ann.origin] = map[common.Hash]struct{}{hash: {}}
				}
			}
			
			if idleWait && len(f.waittime) > 0 {
				f.rescheduleWait(waitTimer, waitTrigger)
			}
			
			
			if !oldPeer && len(f.announces[ann.origin]) > 0 {
				f.scheduleFetches(timeoutTimer, timeoutTrigger, map[string]struct{}{ann.origin: {}})
			}

		case <-waitTrigger:
			
			
			actives := make(map[string]struct{})
			for hash, instance := range f.waittime {
				if time.Duration(f.clock.Now()-instance)+txGatherSlack > txArriveTimeout {
					
					if f.announced[hash] != nil {
						panic("announce tracker already contains waitlist item")
					}
					f.announced[hash] = f.waitlist[hash]
					for peer := range f.waitlist[hash] {
						if announces := f.announces[peer]; announces != nil {
							announces[hash] = struct{}{}
						} else {
							f.announces[peer] = map[common.Hash]struct{}{hash: {}}
						}
						delete(f.waitslots[peer], hash)
						if len(f.waitslots[peer]) == 0 {
							delete(f.waitslots, peer)
						}
						actives[peer] = struct{}{}
					}
					delete(f.waittime, hash)
					delete(f.waitlist, hash)
				}
			}
			
			if len(f.waittime) > 0 {
				f.rescheduleWait(waitTimer, waitTrigger)
			}
			
			if len(actives) > 0 {
				f.scheduleFetches(timeoutTimer, timeoutTrigger, actives)
			}

		case <-timeoutTrigger:
			
			
			
			
			for peer, req := range f.requests {
				if time.Duration(f.clock.Now()-req.time)+txGatherSlack > txFetchTimeout {
					txRequestTimeoutMeter.Mark(int64(len(req.hashes)))

					
					for _, hash := range req.hashes {
						
						if req.stolen != nil {
							if _, ok := req.stolen[hash]; ok {
								continue
							}
						}
						
						if _, ok := f.announced[hash]; ok {
							panic("announced tracker already contains alternate item")
						}
						if f.alternates[hash] != nil { 
							f.announced[hash] = f.alternates[hash]
						}
						delete(f.announced[hash], peer)
						if len(f.announced[hash]) == 0 {
							delete(f.announced, hash)
						}
						delete(f.announces[peer], hash)
						delete(f.alternates, hash)
						delete(f.fetching, hash)
					}
					if len(f.announces[peer]) == 0 {
						delete(f.announces, peer)
					}
					
					f.requests[peer].hashes = nil
				}
			}
			
			f.scheduleFetches(timeoutTimer, timeoutTrigger, nil)

			
			
			f.rescheduleTimeout(timeoutTimer, timeoutTrigger)

		case delivery := <-f.cleanup:
			
			
			for _, hash := range delivery.hashes {
				if _, ok := f.waitlist[hash]; ok {
					for peer, txset := range f.waitslots {
						delete(txset, hash)
						if len(txset) == 0 {
							delete(f.waitslots, peer)
						}
					}
					delete(f.waitlist, hash)
					delete(f.waittime, hash)
				} else {
					for peer, txset := range f.announces {
						delete(txset, hash)
						if len(txset) == 0 {
							delete(f.announces, peer)
						}
					}
					delete(f.announced, hash)
					delete(f.alternates, hash)

					
					
					
					if origin, ok := f.fetching[hash]; ok && (origin != delivery.origin || !delivery.direct) {
						stolen := f.requests[origin].stolen
						if stolen == nil {
							f.requests[origin].stolen = make(map[common.Hash]struct{})
							stolen = f.requests[origin].stolen
						}
						stolen[hash] = struct{}{}
					}
					delete(f.fetching, hash)
				}
			}
			
			
			if delivery.direct {
				
				txRequestDoneMeter.Mark(int64(len(delivery.hashes)))

				
				req := f.requests[delivery.origin]
				if req == nil {
					log.Warn("Unexpected transaction delivery", "peer", delivery.origin)
					break
				}
				delete(f.requests, delivery.origin)

				
				
				delivered := make(map[common.Hash]struct{})
				for _, hash := range delivery.hashes {
					delivered[hash] = struct{}{}
				}
				cutoff := len(req.hashes) 
				for i, hash := range req.hashes {
					if _, ok := delivered[hash]; ok {
						cutoff = i
					}
				}
				
				for i, hash := range req.hashes {
					
					if req.stolen != nil {
						if _, ok := req.stolen[hash]; ok {
							continue
						}
					}
					if _, ok := delivered[hash]; !ok {
						if i < cutoff {
							delete(f.alternates[hash], delivery.origin)
							delete(f.announces[delivery.origin], hash)
							if len(f.announces[delivery.origin]) == 0 {
								delete(f.announces, delivery.origin)
							}
						}
						if len(f.alternates[hash]) > 0 {
							if _, ok := f.announced[hash]; ok {
								panic(fmt.Sprintf("announced tracker already contains alternate item: %v", f.announced[hash]))
							}
							f.announced[hash] = f.alternates[hash]
						}
					}
					delete(f.alternates, hash)
					delete(f.fetching, hash)
				}
				
				f.scheduleFetches(timeoutTimer, timeoutTrigger, nil) 
			}

		case drop := <-f.drop:
			
			if _, ok := f.waitslots[drop.peer]; ok {
				for hash := range f.waitslots[drop.peer] {
					delete(f.waitlist[hash], drop.peer)
					if len(f.waitlist[hash]) == 0 {
						delete(f.waitlist, hash)
						delete(f.waittime, hash)
					}
				}
				delete(f.waitslots, drop.peer)
				if len(f.waitlist) > 0 {
					f.rescheduleWait(waitTimer, waitTrigger)
				}
			}
			
			var request *txRequest
			if request = f.requests[drop.peer]; request != nil {
				for _, hash := range request.hashes {
					
					if request.stolen != nil {
						if _, ok := request.stolen[hash]; ok {
							continue
						}
					}
					
					delete(f.alternates[hash], drop.peer)
					if len(f.alternates[hash]) == 0 {
						delete(f.alternates, hash)
					} else {
						f.announced[hash] = f.alternates[hash]
						delete(f.alternates, hash)
					}
					delete(f.fetching, hash)
				}
				delete(f.requests, drop.peer)
			}
			
			if _, ok := f.announces[drop.peer]; ok {
				for hash := range f.announces[drop.peer] {
					delete(f.announced[hash], drop.peer)
					if len(f.announced[hash]) == 0 {
						delete(f.announced, hash)
					}
				}
				delete(f.announces, drop.peer)
			}
			
			if request != nil {
				f.scheduleFetches(timeoutTimer, timeoutTrigger, nil)
				f.rescheduleTimeout(timeoutTimer, timeoutTrigger)
			}

		case <-f.quit:
			return
		}
		
		txFetcherWaitingPeers.Update(int64(len(f.waitslots)))
		txFetcherWaitingHashes.Update(int64(len(f.waitlist)))
		txFetcherQueueingPeers.Update(int64(len(f.announces) - len(f.requests)))
		txFetcherQueueingHashes.Update(int64(len(f.announced)))
		txFetcherFetchingPeers.Update(int64(len(f.requests)))
		txFetcherFetchingHashes.Update(int64(len(f.fetching)))

		
		if f.step != nil {
			f.step <- struct{}{}
		}
	}
}







func (f *TxFetcher) rescheduleWait(timer *mclock.Timer, trigger chan struct{}) {
	if *timer != nil {
		(*timer).Stop()
	}
	now := f.clock.Now()

	earliest := now
	for _, instance := range f.waittime {
		if earliest > instance {
			earliest = instance
			if txArriveTimeout-time.Duration(now-earliest) < gatherSlack {
				break
			}
		}
	}
	*timer = f.clock.AfterFunc(txArriveTimeout-time.Duration(now-earliest), func() {
		trigger <- struct{}{}
	})
}















func (f *TxFetcher) rescheduleTimeout(timer *mclock.Timer, trigger chan struct{}) {
	if *timer != nil {
		(*timer).Stop()
	}
	now := f.clock.Now()

	earliest := now
	for _, req := range f.requests {
		
		if req.hashes == nil {
			continue
		}
		if earliest > req.time {
			earliest = req.time
			if txFetchTimeout-time.Duration(now-earliest) < gatherSlack {
				break
			}
		}
	}
	*timer = f.clock.AfterFunc(txFetchTimeout-time.Duration(now-earliest), func() {
		trigger <- struct{}{}
	})
}


func (f *TxFetcher) scheduleFetches(timer *mclock.Timer, timeout chan struct{}, whitelist map[string]struct{}) {
	
	actives := whitelist
	if actives == nil {
		actives = make(map[string]struct{})
		for peer := range f.announces {
			actives[peer] = struct{}{}
		}
	}
	if len(actives) == 0 {
		return
	}
	
	idle := len(f.requests) == 0

	f.forEachPeer(actives, func(peer string) {
		if f.requests[peer] != nil {
			return 
		}
		if len(f.announces[peer]) == 0 {
			return 
		}
		hashes := make([]common.Hash, 0, maxTxRetrievals)
		f.forEachHash(f.announces[peer], func(hash common.Hash) bool {
			if _, ok := f.fetching[hash]; !ok {
				
				f.fetching[hash] = peer

				if _, ok := f.alternates[hash]; ok {
					panic(fmt.Sprintf("alternate tracker already contains fetching item: %v", f.alternates[hash]))
				}
				f.alternates[hash] = f.announced[hash]
				delete(f.announced, hash)

				
				hashes = append(hashes, hash)
				if len(hashes) >= maxTxRetrievals {
					return false 
				}
			}
			return true 
		})
		
		if len(hashes) > 0 {
			f.requests[peer] = &txRequest{hashes: hashes, time: f.clock.Now()}
			txRequestOutMeter.Mark(int64(len(hashes)))

			go func(peer string, hashes []common.Hash) {
				
				
				if err := f.fetchTxs(peer, hashes); err != nil {
					txRequestFailMeter.Mark(int64(len(hashes)))
					f.Drop(peer)
				}
			}(peer, hashes)
		}
	})
	
	if idle && len(f.requests) > 0 {
		f.rescheduleTimeout(timer, timeout)
	}
}



func (f *TxFetcher) forEachPeer(peers map[string]struct{}, do func(peer string)) {
	
	if f.rand == nil {
		for peer := range peers {
			do(peer)
		}
		return
	}
	
	list := make([]string, 0, len(peers))
	for peer := range peers {
		list = append(list, peer)
	}
	sort.Strings(list)
	rotateStrings(list, f.rand.Intn(len(list)))
	for _, peer := range list {
		do(peer)
	}
}



func (f *TxFetcher) forEachHash(hashes map[common.Hash]struct{}, do func(hash common.Hash) bool) {
	
	if f.rand == nil {
		for hash := range hashes {
			if !do(hash) {
				return
			}
		}
		return
	}
	
	list := make([]common.Hash, 0, len(hashes))
	for hash := range hashes {
		list = append(list, hash)
	}
	sortHashes(list)
	rotateHashes(list, f.rand.Intn(len(list)))
	for _, hash := range list {
		if !do(hash) {
			return
		}
	}
}



func rotateStrings(slice []string, n int) {
	orig := make([]string, len(slice))
	copy(orig, slice)

	for i := 0; i < len(orig); i++ {
		slice[i] = orig[(i+n)%len(orig)]
	}
}



func sortHashes(slice []common.Hash) {
	for i := 0; i < len(slice); i++ {
		for j := i + 1; j < len(slice); j++ {
			if bytes.Compare(slice[i][:], slice[j][:]) > 0 {
				slice[i], slice[j] = slice[j], slice[i]
			}
		}
	}
}



func rotateHashes(slice []common.Hash, n int) {
	orig := make([]common.Hash, len(slice))
	copy(orig, slice)

	for i := 0; i < len(orig); i++ {
		slice[i] = orig[(i+n)%len(orig)]
	}
}
