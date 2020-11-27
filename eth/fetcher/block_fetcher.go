
















package fetcher

import (
	"errors"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/trie"
)

const (
	lightTimeout  = time.Millisecond       
	arriveTimeout = 500 * time.Millisecond 
	gatherSlack   = 100 * time.Millisecond 
	fetchTimeout  = 5 * time.Second        
)

const (
	maxUncleDist = 7   
	maxQueueDist = 32  
	hashLimit    = 256 
	blockLimit   = 64  
)

var (
	blockAnnounceInMeter   = metrics.NewRegisteredMeter("eth/fetcher/block/announces/in", nil)
	blockAnnounceOutTimer  = metrics.NewRegisteredTimer("eth/fetcher/block/announces/out", nil)
	blockAnnounceDropMeter = metrics.NewRegisteredMeter("eth/fetcher/block/announces/drop", nil)
	blockAnnounceDOSMeter  = metrics.NewRegisteredMeter("eth/fetcher/block/announces/dos", nil)

	blockBroadcastInMeter   = metrics.NewRegisteredMeter("eth/fetcher/block/broadcasts/in", nil)
	blockBroadcastOutTimer  = metrics.NewRegisteredTimer("eth/fetcher/block/broadcasts/out", nil)
	blockBroadcastDropMeter = metrics.NewRegisteredMeter("eth/fetcher/block/broadcasts/drop", nil)
	blockBroadcastDOSMeter  = metrics.NewRegisteredMeter("eth/fetcher/block/broadcasts/dos", nil)

	headerFetchMeter = metrics.NewRegisteredMeter("eth/fetcher/block/headers", nil)
	bodyFetchMeter   = metrics.NewRegisteredMeter("eth/fetcher/block/bodies", nil)

	headerFilterInMeter  = metrics.NewRegisteredMeter("eth/fetcher/block/filter/headers/in", nil)
	headerFilterOutMeter = metrics.NewRegisteredMeter("eth/fetcher/block/filter/headers/out", nil)
	bodyFilterInMeter    = metrics.NewRegisteredMeter("eth/fetcher/block/filter/bodies/in", nil)
	bodyFilterOutMeter   = metrics.NewRegisteredMeter("eth/fetcher/block/filter/bodies/out", nil)
)

var errTerminated = errors.New("terminated")


type HeaderRetrievalFn func(common.Hash) *types.Header


type blockRetrievalFn func(common.Hash) *types.Block


type headerRequesterFn func(common.Hash) error


type bodyRequesterFn func([]common.Hash) error


type headerVerifierFn func(header *types.Header) error


type blockBroadcasterFn func(block *types.Block, propagate bool)


type chainHeightFn func() uint64


type headersInsertFn func(headers []*types.Header) (int, error)


type chainInsertFn func(types.Blocks) (int, error)


type peerDropFn func(id string)



type blockAnnounce struct {
	hash   common.Hash   
	number uint64        
	header *types.Header 
	time   time.Time     

	origin string 

	fetchHeader headerRequesterFn 
	fetchBodies bodyRequesterFn   
}


type headerFilterTask struct {
	peer    string          
	headers []*types.Header 
	time    time.Time       
}



type bodyFilterTask struct {
	peer         string                 
	transactions [][]*types.Transaction 
	uncles       [][]*types.Header      
	time         time.Time              
}


type blockOrHeaderInject struct {
	origin string

	header *types.Header 
	block  *types.Block  
}


func (inject *blockOrHeaderInject) number() uint64 {
	if inject.header != nil {
		return inject.header.Number.Uint64()
	}
	return inject.block.NumberU64()
}


func (inject *blockOrHeaderInject) hash() common.Hash {
	if inject.header != nil {
		return inject.header.Hash()
	}
	return inject.block.Hash()
}



type BlockFetcher struct {
	light bool 

	
	notify chan *blockAnnounce
	inject chan *blockOrHeaderInject

	headerFilter chan chan *headerFilterTask
	bodyFilter   chan chan *bodyFilterTask

	done chan common.Hash
	quit chan struct{}

	
	announces  map[string]int                   
	announced  map[common.Hash][]*blockAnnounce 
	fetching   map[common.Hash]*blockAnnounce   
	fetched    map[common.Hash][]*blockAnnounce 
	completing map[common.Hash]*blockAnnounce   

	
	queue  *prque.Prque                         
	queues map[string]int                       
	queued map[common.Hash]*blockOrHeaderInject 

	
	getHeader      HeaderRetrievalFn  
	getBlock       blockRetrievalFn   
	verifyHeader   headerVerifierFn   
	broadcastBlock blockBroadcasterFn 
	chainHeight    chainHeightFn      
	insertHeaders  headersInsertFn    
	insertChain    chainInsertFn      
	dropPeer       peerDropFn         

	
	announceChangeHook func(common.Hash, bool)           
	queueChangeHook    func(common.Hash, bool)           
	fetchingHook       func([]common.Hash)               
	completingHook     func([]common.Hash)               
	importedHook       func(*types.Header, *types.Block) 
}


func NewBlockFetcher(light bool, getHeader HeaderRetrievalFn, getBlock blockRetrievalFn, verifyHeader headerVerifierFn, broadcastBlock blockBroadcasterFn, chainHeight chainHeightFn, insertHeaders headersInsertFn, insertChain chainInsertFn, dropPeer peerDropFn) *BlockFetcher {
	return &BlockFetcher{
		light:          light,
		notify:         make(chan *blockAnnounce),
		inject:         make(chan *blockOrHeaderInject),
		headerFilter:   make(chan chan *headerFilterTask),
		bodyFilter:     make(chan chan *bodyFilterTask),
		done:           make(chan common.Hash),
		quit:           make(chan struct{}),
		announces:      make(map[string]int),
		announced:      make(map[common.Hash][]*blockAnnounce),
		fetching:       make(map[common.Hash]*blockAnnounce),
		fetched:        make(map[common.Hash][]*blockAnnounce),
		completing:     make(map[common.Hash]*blockAnnounce),
		queue:          prque.New(nil),
		queues:         make(map[string]int),
		queued:         make(map[common.Hash]*blockOrHeaderInject),
		getHeader:      getHeader,
		getBlock:       getBlock,
		verifyHeader:   verifyHeader,
		broadcastBlock: broadcastBlock,
		chainHeight:    chainHeight,
		insertHeaders:  insertHeaders,
		insertChain:    insertChain,
		dropPeer:       dropPeer,
	}
}



func (f *BlockFetcher) Start() {
	go f.loop()
}



func (f *BlockFetcher) Stop() {
	close(f.quit)
}



func (f *BlockFetcher) Notify(peer string, hash common.Hash, number uint64, time time.Time,
	headerFetcher headerRequesterFn, bodyFetcher bodyRequesterFn) error {
	block := &blockAnnounce{
		hash:        hash,
		number:      number,
		time:        time,
		origin:      peer,
		fetchHeader: headerFetcher,
		fetchBodies: bodyFetcher,
	}
	select {
	case f.notify <- block:
		return nil
	case <-f.quit:
		return errTerminated
	}
}


func (f *BlockFetcher) Enqueue(peer string, block *types.Block) error {
	op := &blockOrHeaderInject{
		origin: peer,
		block:  block,
	}
	select {
	case f.inject <- op:
		return nil
	case <-f.quit:
		return errTerminated
	}
}



func (f *BlockFetcher) FilterHeaders(peer string, headers []*types.Header, time time.Time) []*types.Header {
	log.Trace("Filtering headers", "peer", peer, "headers", len(headers))

	
	filter := make(chan *headerFilterTask)

	select {
	case f.headerFilter <- filter:
	case <-f.quit:
		return nil
	}
	
	select {
	case filter <- &headerFilterTask{peer: peer, headers: headers, time: time}:
	case <-f.quit:
		return nil
	}
	
	select {
	case task := <-filter:
		return task.headers
	case <-f.quit:
		return nil
	}
}



func (f *BlockFetcher) FilterBodies(peer string, transactions [][]*types.Transaction, uncles [][]*types.Header, time time.Time) ([][]*types.Transaction, [][]*types.Header) {
	log.Trace("Filtering bodies", "peer", peer, "txs", len(transactions), "uncles", len(uncles))

	
	filter := make(chan *bodyFilterTask)

	select {
	case f.bodyFilter <- filter:
	case <-f.quit:
		return nil, nil
	}
	
	select {
	case filter <- &bodyFilterTask{peer: peer, transactions: transactions, uncles: uncles, time: time}:
	case <-f.quit:
		return nil, nil
	}
	
	select {
	case task := <-filter:
		return task.transactions, task.uncles
	case <-f.quit:
		return nil, nil
	}
}



func (f *BlockFetcher) loop() {
	
	fetchTimer := time.NewTimer(0)
	completeTimer := time.NewTimer(0)
	defer fetchTimer.Stop()
	defer completeTimer.Stop()

	for {
		
		for hash, announce := range f.fetching {
			if time.Since(announce.time) > fetchTimeout {
				f.forgetHash(hash)
			}
		}
		
		height := f.chainHeight()
		for !f.queue.Empty() {
			op := f.queue.PopItem().(*blockOrHeaderInject)
			hash := op.hash()
			if f.queueChangeHook != nil {
				f.queueChangeHook(hash, false)
			}
			
			number := op.number()
			if number > height+1 {
				f.queue.Push(op, -int64(number))
				if f.queueChangeHook != nil {
					f.queueChangeHook(hash, true)
				}
				break
			}
			
			if (number+maxUncleDist < height) || (f.light && f.getHeader(hash) != nil) || (!f.light && f.getBlock(hash) != nil) {
				f.forgetBlock(hash)
				continue
			}
			if f.light {
				f.importHeaders(op.origin, op.header)
			} else {
				f.importBlocks(op.origin, op.block)
			}
		}
		
		select {
		case <-f.quit:
			
			return

		case notification := <-f.notify:
			
			blockAnnounceInMeter.Mark(1)

			count := f.announces[notification.origin] + 1
			if count > hashLimit {
				log.Debug("Peer exceeded outstanding announces", "peer", notification.origin, "limit", hashLimit)
				blockAnnounceDOSMeter.Mark(1)
				break
			}
			
			if notification.number > 0 {
				if dist := int64(notification.number) - int64(f.chainHeight()); dist < -maxUncleDist || dist > maxQueueDist {
					log.Debug("Peer discarded announcement", "peer", notification.origin, "number", notification.number, "hash", notification.hash, "distance", dist)
					blockAnnounceDropMeter.Mark(1)
					break
				}
			}
			
			if _, ok := f.fetching[notification.hash]; ok {
				break
			}
			if _, ok := f.completing[notification.hash]; ok {
				break
			}
			f.announces[notification.origin] = count
			f.announced[notification.hash] = append(f.announced[notification.hash], notification)
			if f.announceChangeHook != nil && len(f.announced[notification.hash]) == 1 {
				f.announceChangeHook(notification.hash, true)
			}
			if len(f.announced) == 1 {
				f.rescheduleFetch(fetchTimer)
			}

		case op := <-f.inject:
			
			blockBroadcastInMeter.Mark(1)

			
			
			if f.light {
				continue
			}
			f.enqueue(op.origin, nil, op.block)

		case hash := <-f.done:
			
			f.forgetHash(hash)
			f.forgetBlock(hash)

		case <-fetchTimer.C:
			
			request := make(map[string][]common.Hash)

			for hash, announces := range f.announced {
				
				
				timeout := arriveTimeout - gatherSlack
				if f.light {
					timeout = 0
				}
				if time.Since(announces[0].time) > timeout {
					
					announce := announces[rand.Intn(len(announces))]
					f.forgetHash(hash)

					
					if (f.light && f.getHeader(hash) == nil) || (!f.light && f.getBlock(hash) == nil) {
						request[announce.origin] = append(request[announce.origin], hash)
						f.fetching[hash] = announce
					}
				}
			}
			
			for peer, hashes := range request {
				log.Trace("Fetching scheduled headers", "peer", peer, "list", hashes)

				
				fetchHeader, hashes := f.fetching[hashes[0]].fetchHeader, hashes
				go func() {
					if f.fetchingHook != nil {
						f.fetchingHook(hashes)
					}
					for _, hash := range hashes {
						headerFetchMeter.Mark(1)
						fetchHeader(hash) 
					}
				}()
			}
			
			f.rescheduleFetch(fetchTimer)

		case <-completeTimer.C:
			
			request := make(map[string][]common.Hash)

			for hash, announces := range f.fetched {
				
				announce := announces[rand.Intn(len(announces))]
				f.forgetHash(hash)

				
				if f.getBlock(hash) == nil {
					request[announce.origin] = append(request[announce.origin], hash)
					f.completing[hash] = announce
				}
			}
			
			for peer, hashes := range request {
				log.Trace("Fetching scheduled bodies", "peer", peer, "list", hashes)

				
				if f.completingHook != nil {
					f.completingHook(hashes)
				}
				bodyFetchMeter.Mark(int64(len(hashes)))
				go f.completing[hashes[0]].fetchBodies(hashes)
			}
			
			f.rescheduleComplete(completeTimer)

		case filter := <-f.headerFilter:
			
			
			
			var task *headerFilterTask
			select {
			case task = <-filter:
			case <-f.quit:
				return
			}
			headerFilterInMeter.Mark(int64(len(task.headers)))

			
			
			unknown, incomplete, complete, lightHeaders := []*types.Header{}, []*blockAnnounce{}, []*types.Block{}, []*blockAnnounce{}
			for _, header := range task.headers {
				hash := header.Hash()

				
				if announce := f.fetching[hash]; announce != nil && announce.origin == task.peer && f.fetched[hash] == nil && f.completing[hash] == nil && f.queued[hash] == nil {
					
					if header.Number.Uint64() != announce.number {
						log.Trace("Invalid block number fetched", "peer", announce.origin, "hash", header.Hash(), "announced", announce.number, "provided", header.Number)
						f.dropPeer(announce.origin)
						f.forgetHash(hash)
						continue
					}
					
					
					if f.light {
						if f.getHeader(hash) == nil {
							announce.header = header
							lightHeaders = append(lightHeaders, announce)
						}
						f.forgetHash(hash)
						continue
					}
					
					if f.getBlock(hash) == nil {
						announce.header = header
						announce.time = task.time

						
						if header.TxHash == types.EmptyRootHash && header.UncleHash == types.EmptyUncleHash {
							log.Trace("Block empty, skipping body retrieval", "peer", announce.origin, "number", header.Number, "hash", header.Hash())

							block := types.NewBlockWithHeader(header)
							block.ReceivedAt = task.time

							complete = append(complete, block)
							f.completing[hash] = announce
							continue
						}
						
						incomplete = append(incomplete, announce)
					} else {
						log.Trace("Block already imported, discarding header", "peer", announce.origin, "number", header.Number, "hash", header.Hash())
						f.forgetHash(hash)
					}
				} else {
					
					unknown = append(unknown, header)
				}
			}
			headerFilterOutMeter.Mark(int64(len(unknown)))
			select {
			case filter <- &headerFilterTask{headers: unknown, time: task.time}:
			case <-f.quit:
				return
			}
			
			for _, announce := range incomplete {
				hash := announce.header.Hash()
				if _, ok := f.completing[hash]; ok {
					continue
				}
				f.fetched[hash] = append(f.fetched[hash], announce)
				if len(f.fetched) == 1 {
					f.rescheduleComplete(completeTimer)
				}
			}
			
			for _, announce := range lightHeaders {
				f.enqueue(announce.origin, announce.header, nil)
			}
			
			for _, block := range complete {
				if announce := f.completing[block.Hash()]; announce != nil {
					f.enqueue(announce.origin, nil, block)
				}
			}

		case filter := <-f.bodyFilter:
			
			var task *bodyFilterTask
			select {
			case task = <-filter:
			case <-f.quit:
				return
			}
			bodyFilterInMeter.Mark(int64(len(task.transactions)))
			blocks := []*types.Block{}
			
			if len(f.completing) > 0 {
				for i := 0; i < len(task.transactions) && i < len(task.uncles); i++ {
					
					var (
						matched   = false
						uncleHash common.Hash 
						txnHash   common.Hash 
					)
					for hash, announce := range f.completing {
						if f.queued[hash] != nil || announce.origin != task.peer {
							continue
						}
						if uncleHash == (common.Hash{}) {
							uncleHash = types.CalcUncleHash(task.uncles[i])
						}
						if uncleHash != announce.header.UncleHash {
							continue
						}
						if txnHash == (common.Hash{}) {
							txnHash = types.DeriveSha(types.Transactions(task.transactions[i]), new(trie.Trie))
						}
						if txnHash != announce.header.TxHash {
							continue
						}
						
						matched = true
						if f.getBlock(hash) == nil {
							block := types.NewBlockWithHeader(announce.header).WithBody(task.transactions[i], task.uncles[i])
							block.ReceivedAt = task.time
							blocks = append(blocks, block)
						} else {
							f.forgetHash(hash)
						}

					}
					if matched {
						task.transactions = append(task.transactions[:i], task.transactions[i+1:]...)
						task.uncles = append(task.uncles[:i], task.uncles[i+1:]...)
						i--
						continue
					}
				}
			}
			bodyFilterOutMeter.Mark(int64(len(task.transactions)))
			select {
			case filter <- task:
			case <-f.quit:
				return
			}
			
			for _, block := range blocks {
				if announce := f.completing[block.Hash()]; announce != nil {
					f.enqueue(announce.origin, nil, block)
				}
			}
		}
	}
}


func (f *BlockFetcher) rescheduleFetch(fetch *time.Timer) {
	
	if len(f.announced) == 0 {
		return
	}
	
	
	if f.light {
		fetch.Reset(lightTimeout)
		return
	}
	
	earliest := time.Now()
	for _, announces := range f.announced {
		if earliest.After(announces[0].time) {
			earliest = announces[0].time
		}
	}
	fetch.Reset(arriveTimeout - time.Since(earliest))
}


func (f *BlockFetcher) rescheduleComplete(complete *time.Timer) {
	
	if len(f.fetched) == 0 {
		return
	}
	
	earliest := time.Now()
	for _, announces := range f.fetched {
		if earliest.After(announces[0].time) {
			earliest = announces[0].time
		}
	}
	complete.Reset(gatherSlack - time.Since(earliest))
}



func (f *BlockFetcher) enqueue(peer string, header *types.Header, block *types.Block) {
	var (
		hash   common.Hash
		number uint64
	)
	if header != nil {
		hash, number = header.Hash(), header.Number.Uint64()
	} else {
		hash, number = block.Hash(), block.NumberU64()
	}
	
	count := f.queues[peer] + 1
	if count > blockLimit {
		log.Debug("Discarded delivered header or block, exceeded allowance", "peer", peer, "number", number, "hash", hash, "limit", blockLimit)
		blockBroadcastDOSMeter.Mark(1)
		f.forgetHash(hash)
		return
	}
	
	if dist := int64(number) - int64(f.chainHeight()); dist < -maxUncleDist || dist > maxQueueDist {
		log.Debug("Discarded delivered header or block, too far away", "peer", peer, "number", number, "hash", hash, "distance", dist)
		blockBroadcastDropMeter.Mark(1)
		f.forgetHash(hash)
		return
	}
	
	if _, ok := f.queued[hash]; !ok {
		op := &blockOrHeaderInject{origin: peer}
		if header != nil {
			op.header = header
		} else {
			op.block = block
		}
		f.queues[peer] = count
		f.queued[hash] = op
		f.queue.Push(op, -int64(number))
		if f.queueChangeHook != nil {
			f.queueChangeHook(hash, true)
		}
		log.Debug("Queued delivered header or block", "peer", peer, "number", number, "hash", hash, "queued", f.queue.Size())
	}
}




func (f *BlockFetcher) importHeaders(peer string, header *types.Header) {
	hash := header.Hash()
	log.Debug("Importing propagated header", "peer", peer, "number", header.Number, "hash", hash)

	go func() {
		defer func() { f.done <- hash }()
		
		parent := f.getHeader(header.ParentHash)
		if parent == nil {
			log.Debug("Unknown parent of propagated header", "peer", peer, "number", header.Number, "hash", hash, "parent", header.ParentHash)
			return
		}
		
		if err := f.verifyHeader(header); err != nil && err != consensus.ErrFutureBlock {
			log.Debug("Propagated header verification failed", "peer", peer, "number", header.Number, "hash", hash, "err", err)
			f.dropPeer(peer)
			return
		}
		
		if _, err := f.insertHeaders([]*types.Header{header}); err != nil {
			log.Debug("Propagated header import failed", "peer", peer, "number", header.Number, "hash", hash, "err", err)
			return
		}
		
		if f.importedHook != nil {
			f.importedHook(header, nil)
		}
	}()
}




func (f *BlockFetcher) importBlocks(peer string, block *types.Block) {
	hash := block.Hash()

	
	log.Debug("Importing propagated block", "peer", peer, "number", block.Number(), "hash", hash)
	go func() {
		defer func() { f.done <- hash }()

		
		parent := f.getBlock(block.ParentHash())
		if parent == nil {
			log.Debug("Unknown parent of propagated block", "peer", peer, "number", block.Number(), "hash", hash, "parent", block.ParentHash())
			return
		}
		
		switch err := f.verifyHeader(block.Header()); err {
		case nil:
			
			blockBroadcastOutTimer.UpdateSince(block.ReceivedAt)
			go f.broadcastBlock(block, true)

		case consensus.ErrFutureBlock:
			

		default:
			
			log.Debug("Propagated block verification failed", "peer", peer, "number", block.Number(), "hash", hash, "err", err)
			f.dropPeer(peer)
			return
		}
		
		if _, err := f.insertChain(types.Blocks{block}); err != nil {
			log.Debug("Propagated block import failed", "peer", peer, "number", block.Number(), "hash", hash, "err", err)
			return
		}
		
		blockAnnounceOutTimer.UpdateSince(block.ReceivedAt)
		go f.broadcastBlock(block, false)

		
		if f.importedHook != nil {
			f.importedHook(nil, block)
		}
	}()
}



func (f *BlockFetcher) forgetHash(hash common.Hash) {
	
	for _, announce := range f.announced[hash] {
		f.announces[announce.origin]--
		if f.announces[announce.origin] <= 0 {
			delete(f.announces, announce.origin)
		}
	}
	delete(f.announced, hash)
	if f.announceChangeHook != nil {
		f.announceChangeHook(hash, false)
	}
	
	if announce := f.fetching[hash]; announce != nil {
		f.announces[announce.origin]--
		if f.announces[announce.origin] <= 0 {
			delete(f.announces, announce.origin)
		}
		delete(f.fetching, hash)
	}

	
	for _, announce := range f.fetched[hash] {
		f.announces[announce.origin]--
		if f.announces[announce.origin] <= 0 {
			delete(f.announces, announce.origin)
		}
	}
	delete(f.fetched, hash)

	
	if announce := f.completing[hash]; announce != nil {
		f.announces[announce.origin]--
		if f.announces[announce.origin] <= 0 {
			delete(f.announces, announce.origin)
		}
		delete(f.completing, hash)
	}
}



func (f *BlockFetcher) forgetBlock(hash common.Hash) {
	if insert := f.queued[hash]; insert != nil {
		f.queues[insert.origin]--
		if f.queues[insert.origin] == 0 {
			delete(f.queues, insert.origin)
		}
		delete(f.queued, hash)
	}
}
