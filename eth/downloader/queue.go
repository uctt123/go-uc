


















package downloader

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/trie"
)

const (
	bodyType    = uint(0)
	receiptType = uint(1)
)

var (
	blockCacheMaxItems     = 8192             
	blockCacheInitialItems = 2048             
	blockCacheMemory       = 64 * 1024 * 1024 
	blockCacheSizeWeight   = 0.1              
)

var (
	errNoFetchesPending = errors.New("no fetches pending")
	errStaleDelivery    = errors.New("stale delivery")
)


type fetchRequest struct {
	Peer    *peerConnection 
	From    uint64          
	Headers []*types.Header 
	Time    time.Time       
}



type fetchResult struct {
	pending int32 

	Header       *types.Header
	Uncles       []*types.Header
	Transactions types.Transactions
	Receipts     types.Receipts
}

func newFetchResult(header *types.Header, fastSync bool) *fetchResult {
	item := &fetchResult{
		Header: header,
	}
	if !header.EmptyBody() {
		item.pending |= (1 << bodyType)
	}
	if fastSync && !header.EmptyReceipts() {
		item.pending |= (1 << receiptType)
	}
	return item
}


func (f *fetchResult) SetBodyDone() {
	if v := atomic.LoadInt32(&f.pending); (v & (1 << bodyType)) != 0 {
		atomic.AddInt32(&f.pending, -1)
	}
}


func (f *fetchResult) AllDone() bool {
	return atomic.LoadInt32(&f.pending) == 0
}


func (f *fetchResult) SetReceiptsDone() {
	if v := atomic.LoadInt32(&f.pending); (v & (1 << receiptType)) != 0 {
		atomic.AddInt32(&f.pending, -2)
	}
}


func (f *fetchResult) Done(kind uint) bool {
	v := atomic.LoadInt32(&f.pending)
	return v&(1<<kind) == 0
}


type queue struct {
	mode SyncMode 

	
	headerHead      common.Hash                    
	headerTaskPool  map[uint64]*types.Header       
	headerTaskQueue *prque.Prque                   
	headerPeerMiss  map[string]map[uint64]struct{} 
	headerPendPool  map[string]*fetchRequest       
	headerResults   []*types.Header                
	headerProced    int                            
	headerOffset    uint64                         
	headerContCh    chan bool                      

	
	blockTaskPool  map[common.Hash]*types.Header 
	blockTaskQueue *prque.Prque                  
	blockPendPool  map[string]*fetchRequest      

	receiptTaskPool  map[common.Hash]*types.Header 
	receiptTaskQueue *prque.Prque                  
	receiptPendPool  map[string]*fetchRequest      

	resultCache *resultStore       
	resultSize  common.StorageSize 

	lock   *sync.RWMutex
	active *sync.Cond
	closed bool

	lastStatLog time.Time
}


func newQueue(blockCacheLimit int, thresholdInitialSize int) *queue {
	lock := new(sync.RWMutex)
	q := &queue{
		headerContCh:     make(chan bool),
		blockTaskQueue:   prque.New(nil),
		receiptTaskQueue: prque.New(nil),
		active:           sync.NewCond(lock),
		lock:             lock,
	}
	q.Reset(blockCacheLimit, thresholdInitialSize)
	return q
}


func (q *queue) Reset(blockCacheLimit int, thresholdInitialSize int) {
	q.lock.Lock()
	defer q.lock.Unlock()

	q.closed = false
	q.mode = FullSync

	q.headerHead = common.Hash{}
	q.headerPendPool = make(map[string]*fetchRequest)

	q.blockTaskPool = make(map[common.Hash]*types.Header)
	q.blockTaskQueue.Reset()
	q.blockPendPool = make(map[string]*fetchRequest)

	q.receiptTaskPool = make(map[common.Hash]*types.Header)
	q.receiptTaskQueue.Reset()
	q.receiptPendPool = make(map[string]*fetchRequest)

	q.resultCache = newResultStore(blockCacheLimit)
	q.resultCache.SetThrottleThreshold(uint64(thresholdInitialSize))
}



func (q *queue) Close() {
	q.lock.Lock()
	q.closed = true
	q.active.Signal()
	q.lock.Unlock()
}


func (q *queue) PendingHeaders() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.headerTaskQueue.Size()
}


func (q *queue) PendingBlocks() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.blockTaskQueue.Size()
}


func (q *queue) PendingReceipts() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.receiptTaskQueue.Size()
}



func (q *queue) InFlightHeaders() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.headerPendPool) > 0
}



func (q *queue) InFlightBlocks() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.blockPendPool) > 0
}



func (q *queue) InFlightReceipts() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.receiptPendPool) > 0
}


func (q *queue) Idle() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	queued := q.blockTaskQueue.Size() + q.receiptTaskQueue.Size()
	pending := len(q.blockPendPool) + len(q.receiptPendPool)

	return (queued + pending) == 0
}



func (q *queue) ScheduleSkeleton(from uint64, skeleton []*types.Header) {
	q.lock.Lock()
	defer q.lock.Unlock()

	
	if q.headerResults != nil {
		panic("skeleton assembly already in progress")
	}
	
	q.headerTaskPool = make(map[uint64]*types.Header)
	q.headerTaskQueue = prque.New(nil)
	q.headerPeerMiss = make(map[string]map[uint64]struct{}) 
	q.headerResults = make([]*types.Header, len(skeleton)*MaxHeaderFetch)
	q.headerProced = 0
	q.headerOffset = from
	q.headerContCh = make(chan bool, 1)

	for i, header := range skeleton {
		index := from + uint64(i*MaxHeaderFetch)

		q.headerTaskPool[index] = header
		q.headerTaskQueue.Push(index, -int64(index))
	}
}



func (q *queue) RetrieveHeaders() ([]*types.Header, int) {
	q.lock.Lock()
	defer q.lock.Unlock()

	headers, proced := q.headerResults, q.headerProced
	q.headerResults, q.headerProced = nil, 0

	return headers, proced
}



func (q *queue) Schedule(headers []*types.Header, from uint64) []*types.Header {
	q.lock.Lock()
	defer q.lock.Unlock()

	
	inserts := make([]*types.Header, 0, len(headers))
	for _, header := range headers {
		
		hash := header.Hash()
		if header.Number == nil || header.Number.Uint64() != from {
			log.Warn("Header broke chain ordering", "number", header.Number, "hash", hash, "expected", from)
			break
		}
		if q.headerHead != (common.Hash{}) && q.headerHead != header.ParentHash {
			log.Warn("Header broke chain ancestry", "number", header.Number, "hash", hash)
			break
		}
		
		
		
		if _, ok := q.blockTaskPool[hash]; ok {
			log.Warn("Header already scheduled for block fetch", "number", header.Number, "hash", hash)
		} else {
			q.blockTaskPool[hash] = header
			q.blockTaskQueue.Push(header, -int64(header.Number.Uint64()))
		}
		
		if q.mode == FastSync && !header.EmptyReceipts() {
			if _, ok := q.receiptTaskPool[hash]; ok {
				log.Warn("Header already scheduled for receipt fetch", "number", header.Number, "hash", hash)
			} else {
				q.receiptTaskPool[hash] = header
				q.receiptTaskQueue.Push(header, -int64(header.Number.Uint64()))
			}
		}
		inserts = append(inserts, header)
		q.headerHead = hash
		from++
	}
	return inserts
}





func (q *queue) Results(block bool) []*fetchResult {
	
	if !block && !q.resultCache.HasCompletedItems() {
		return nil
	}
	closed := false
	for !closed && !q.resultCache.HasCompletedItems() {
		
		
		
		
		
		
		
		q.lock.Lock()
		if q.resultCache.HasCompletedItems() || q.closed {
			q.lock.Unlock()
			break
		}
		
		q.active.Wait()
		closed = q.closed
		q.lock.Unlock()
	}
	
	results := q.resultCache.GetCompleted(maxResultsProcess)
	for _, result := range results {
		
		size := result.Header.Size()
		for _, uncle := range result.Uncles {
			size += uncle.Size()
		}
		for _, receipt := range result.Receipts {
			size += receipt.Size()
		}
		for _, tx := range result.Transactions {
			size += tx.Size()
		}
		q.resultSize = common.StorageSize(blockCacheSizeWeight)*size +
			(1-common.StorageSize(blockCacheSizeWeight))*q.resultSize
	}
	
	
	throttleThreshold := uint64((common.StorageSize(blockCacheMemory) + q.resultSize - 1) / q.resultSize)
	throttleThreshold = q.resultCache.SetThrottleThreshold(throttleThreshold)

	
	if time.Since(q.lastStatLog) > 60*time.Second {
		q.lastStatLog = time.Now()
		info := q.Stats()
		info = append(info, "throttle", throttleThreshold)
		log.Info("Downloader queue stats", info...)
	}
	return results
}

func (q *queue) Stats() []interface{} {
	q.lock.RLock()
	defer q.lock.RUnlock()

	return q.stats()
}

func (q *queue) stats() []interface{} {
	return []interface{}{
		"receiptTasks", q.receiptTaskQueue.Size(),
		"blockTasks", q.blockTaskQueue.Size(),
		"itemSize", q.resultSize,
	}
}



func (q *queue) ReserveHeaders(p *peerConnection, count int) *fetchRequest {
	q.lock.Lock()
	defer q.lock.Unlock()

	
	
	if _, ok := q.headerPendPool[p.id]; ok {
		return nil
	}
	
	send, skip := uint64(0), []uint64{}
	for send == 0 && !q.headerTaskQueue.Empty() {
		from, _ := q.headerTaskQueue.Pop()
		if q.headerPeerMiss[p.id] != nil {
			if _, ok := q.headerPeerMiss[p.id][from.(uint64)]; ok {
				skip = append(skip, from.(uint64))
				continue
			}
		}
		send = from.(uint64)
	}
	
	for _, from := range skip {
		q.headerTaskQueue.Push(from, -int64(from))
	}
	
	if send == 0 {
		return nil
	}
	request := &fetchRequest{
		Peer: p,
		From: send,
		Time: time.Now(),
	}
	q.headerPendPool[p.id] = request
	return request
}




func (q *queue) ReserveBodies(p *peerConnection, count int) (*fetchRequest, bool, bool) {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.reserveHeaders(p, count, q.blockTaskPool, q.blockTaskQueue, q.blockPendPool, bodyType)
}




func (q *queue) ReserveReceipts(p *peerConnection, count int) (*fetchRequest, bool, bool) {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.reserveHeaders(p, count, q.receiptTaskPool, q.receiptTaskQueue, q.receiptPendPool, receiptType)
}













func (q *queue) reserveHeaders(p *peerConnection, count int, taskPool map[common.Hash]*types.Header, taskQueue *prque.Prque,
	pendPool map[string]*fetchRequest, kind uint) (*fetchRequest, bool, bool) {
	
	
	if taskQueue.Empty() {
		return nil, false, true
	}
	if _, ok := pendPool[p.id]; ok {
		return nil, false, false
	}
	
	send := make([]*types.Header, 0, count)
	skip := make([]*types.Header, 0)
	progress := false
	throttled := false
	for proc := 0; len(send) < count && !taskQueue.Empty(); proc++ {
		
		
		h, _ := taskQueue.Peek()
		header := h.(*types.Header)
		
		

		stale, throttle, item, err := q.resultCache.AddFetch(header, q.mode == FastSync)
		if stale {
			
			
			taskQueue.PopItem()
			progress = true
			delete(taskPool, header.Hash())
			proc = proc - 1
			log.Error("Fetch reservation already delivered", "number", header.Number.Uint64())
			continue
		}
		if throttle {
			
			
			
			
			throttled = len(skip) == 0
			break
		}
		if err != nil {
			
			log.Warn("Failed to reserve headers", "err", err)
			
			break
		}
		if item.Done(kind) {
			
			delete(taskPool, header.Hash())
			taskQueue.PopItem()
			proc = proc - 1
			progress = true
			continue
		}
		
		taskQueue.PopItem()
		
		if p.Lacks(header.Hash()) {
			skip = append(skip, header)
		} else {
			send = append(send, header)
		}
	}
	
	for _, header := range skip {
		taskQueue.Push(header, -int64(header.Number.Uint64()))
	}
	if q.resultCache.HasCompletedItems() {
		
		q.active.Signal()
	}
	
	if len(send) == 0 {
		return nil, progress, throttled
	}
	request := &fetchRequest{
		Peer:    p,
		Headers: send,
		Time:    time.Now(),
	}
	pendPool[p.id] = request
	return request, progress, throttled
}


func (q *queue) CancelHeaders(request *fetchRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.cancel(request, q.headerTaskQueue, q.headerPendPool)
}



func (q *queue) CancelBodies(request *fetchRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.cancel(request, q.blockTaskQueue, q.blockPendPool)
}



func (q *queue) CancelReceipts(request *fetchRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.cancel(request, q.receiptTaskQueue, q.receiptPendPool)
}


func (q *queue) cancel(request *fetchRequest, taskQueue *prque.Prque, pendPool map[string]*fetchRequest) {
	if request.From > 0 {
		taskQueue.Push(request.From, -int64(request.From))
	}
	for _, header := range request.Headers {
		taskQueue.Push(header, -int64(header.Number.Uint64()))
	}
	delete(pendPool, request.Peer.id)
}




func (q *queue) Revoke(peerID string) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if request, ok := q.blockPendPool[peerID]; ok {
		for _, header := range request.Headers {
			q.blockTaskQueue.Push(header, -int64(header.Number.Uint64()))
		}
		delete(q.blockPendPool, peerID)
	}
	if request, ok := q.receiptPendPool[peerID]; ok {
		for _, header := range request.Headers {
			q.receiptTaskQueue.Push(header, -int64(header.Number.Uint64()))
		}
		delete(q.receiptPendPool, peerID)
	}
}



func (q *queue) ExpireHeaders(timeout time.Duration) map[string]int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.expire(timeout, q.headerPendPool, q.headerTaskQueue, headerTimeoutMeter)
}



func (q *queue) ExpireBodies(timeout time.Duration) map[string]int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.expire(timeout, q.blockPendPool, q.blockTaskQueue, bodyTimeoutMeter)
}



func (q *queue) ExpireReceipts(timeout time.Duration) map[string]int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.expire(timeout, q.receiptPendPool, q.receiptTaskQueue, receiptTimeoutMeter)
}







func (q *queue) expire(timeout time.Duration, pendPool map[string]*fetchRequest, taskQueue *prque.Prque, timeoutMeter metrics.Meter) map[string]int {
	
	expiries := make(map[string]int)
	for id, request := range pendPool {
		if time.Since(request.Time) > timeout {
			
			timeoutMeter.Mark(1)

			
			if request.From > 0 {
				taskQueue.Push(request.From, -int64(request.From))
			}
			for _, header := range request.Headers {
				taskQueue.Push(header, -int64(header.Number.Uint64()))
			}
			
			expiries[id] = len(request.Headers)

			
			delete(pendPool, id)
		}
	}
	return expiries
}








func (q *queue) DeliverHeaders(id string, headers []*types.Header, headerProcCh chan []*types.Header) (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	
	request := q.headerPendPool[id]
	if request == nil {
		return 0, errNoFetchesPending
	}
	headerReqTimer.UpdateSince(request.Time)
	delete(q.headerPendPool, id)

	
	target := q.headerTaskPool[request.From].Hash()

	accepted := len(headers) == MaxHeaderFetch
	if accepted {
		if headers[0].Number.Uint64() != request.From {
			log.Trace("First header broke chain ordering", "peer", id, "number", headers[0].Number, "hash", headers[0].Hash(), request.From)
			accepted = false
		} else if headers[len(headers)-1].Hash() != target {
			log.Trace("Last header broke skeleton structure ", "peer", id, "number", headers[len(headers)-1].Number, "hash", headers[len(headers)-1].Hash(), "expected", target)
			accepted = false
		}
	}
	if accepted {
		parentHash := headers[0].Hash()
		for i, header := range headers[1:] {
			hash := header.Hash()
			if want := request.From + 1 + uint64(i); header.Number.Uint64() != want {
				log.Warn("Header broke chain ordering", "peer", id, "number", header.Number, "hash", hash, "expected", want)
				accepted = false
				break
			}
			if parentHash != header.ParentHash {
				log.Warn("Header broke chain ancestry", "peer", id, "number", header.Number, "hash", hash)
				accepted = false
				break
			}
			
			parentHash = hash
		}
	}
	
	if !accepted {
		log.Trace("Skeleton filling not accepted", "peer", id, "from", request.From)

		miss := q.headerPeerMiss[id]
		if miss == nil {
			q.headerPeerMiss[id] = make(map[uint64]struct{})
			miss = q.headerPeerMiss[id]
		}
		miss[request.From] = struct{}{}

		q.headerTaskQueue.Push(request.From, -int64(request.From))
		return 0, errors.New("delivery not accepted")
	}
	
	copy(q.headerResults[request.From-q.headerOffset:], headers)
	delete(q.headerTaskPool, request.From)

	ready := 0
	for q.headerProced+ready < len(q.headerResults) && q.headerResults[q.headerProced+ready] != nil {
		ready += MaxHeaderFetch
	}
	if ready > 0 {
		
		process := make([]*types.Header, ready)
		copy(process, q.headerResults[q.headerProced:q.headerProced+ready])

		select {
		case headerProcCh <- process:
			log.Trace("Pre-scheduled new headers", "peer", id, "count", len(process), "from", process[0].Number)
			q.headerProced += len(process)
		default:
		}
	}
	
	if len(q.headerTaskPool) == 0 {
		q.headerContCh <- false
	}
	return len(headers), nil
}




func (q *queue) DeliverBodies(id string, txLists [][]*types.Transaction, uncleLists [][]*types.Header) (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()
	validate := func(index int, header *types.Header) error {
		if types.DeriveSha(types.Transactions(txLists[index]), trie.NewStackTrie(nil)) != header.TxHash {
			return errInvalidBody
		}
		if types.CalcUncleHash(uncleLists[index]) != header.UncleHash {
			return errInvalidBody
		}
		return nil
	}

	reconstruct := func(index int, result *fetchResult) {
		result.Transactions = txLists[index]
		result.Uncles = uncleLists[index]
		result.SetBodyDone()
	}
	return q.deliver(id, q.blockTaskPool, q.blockTaskQueue, q.blockPendPool,
		bodyReqTimer, len(txLists), validate, reconstruct)
}




func (q *queue) DeliverReceipts(id string, receiptList [][]*types.Receipt) (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()
	validate := func(index int, header *types.Header) error {
		if types.DeriveSha(types.Receipts(receiptList[index]), trie.NewStackTrie(nil)) != header.ReceiptHash {
			return errInvalidReceipt
		}
		return nil
	}
	reconstruct := func(index int, result *fetchResult) {
		result.Receipts = receiptList[index]
		result.SetReceiptsDone()
	}
	return q.deliver(id, q.receiptTaskPool, q.receiptTaskQueue, q.receiptPendPool,
		receiptReqTimer, len(receiptList), validate, reconstruct)
}






func (q *queue) deliver(id string, taskPool map[common.Hash]*types.Header,
	taskQueue *prque.Prque, pendPool map[string]*fetchRequest, reqTimer metrics.Timer,
	results int, validate func(index int, header *types.Header) error,
	reconstruct func(index int, result *fetchResult)) (int, error) {

	
	request := pendPool[id]
	if request == nil {
		return 0, errNoFetchesPending
	}
	reqTimer.UpdateSince(request.Time)
	delete(pendPool, id)

	
	if results == 0 {
		for _, header := range request.Headers {
			request.Peer.MarkLacking(header.Hash())
		}
	}
	
	var (
		accepted int
		failure  error
		i        int
		hashes   []common.Hash
	)
	for _, header := range request.Headers {
		
		if i >= results {
			break
		}
		
		if err := validate(i, header); err != nil {
			failure = err
			break
		}
		hashes = append(hashes, header.Hash())
		i++
	}

	for _, header := range request.Headers[:i] {
		if res, stale, err := q.resultCache.GetDeliverySlot(header.Number.Uint64()); err == nil {
			reconstruct(accepted, res)
		} else {
			
			
			
			log.Error("Delivery stale", "stale", stale, "number", header.Number.Uint64(), "err", err)
			failure = errStaleDelivery
		}
		
		delete(taskPool, hashes[accepted])
		accepted++
	}
	
	for _, header := range request.Headers[accepted:] {
		taskQueue.Push(header, -int64(header.Number.Uint64()))
	}
	
	if accepted > 0 {
		q.active.Signal()
	}
	if failure == nil {
		return accepted, nil
	}
	
	if errors.Is(failure, errInvalidChain) {
		return accepted, failure
	}
	if accepted > 0 {
		return accepted, fmt.Errorf("partial failure: %v", failure)
	}
	return accepted, fmt.Errorf("%w: %v", failure, errStaleDelivery)
}



func (q *queue) Prepare(offset uint64, mode SyncMode) {
	q.lock.Lock()
	defer q.lock.Unlock()

	
	q.resultCache.Prepare(offset)
	q.mode = mode
}
