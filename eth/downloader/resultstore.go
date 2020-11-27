















package downloader

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/core/types"
)



type resultStore struct {
	items        []*fetchResult 
	resultOffset uint64         

	
	
	
	indexIncomplete int32 

	
	
	
	
	
	throttleThreshold uint64

	lock sync.RWMutex
}

func newResultStore(size int) *resultStore {
	return &resultStore{
		resultOffset:      0,
		items:             make([]*fetchResult, size),
		throttleThreshold: uint64(size),
	}
}



func (r *resultStore) SetThrottleThreshold(threshold uint64) uint64 {
	r.lock.Lock()
	defer r.lock.Unlock()

	limit := uint64(len(r.items))
	if threshold >= limit {
		threshold = limit
	}
	r.throttleThreshold = threshold
	return r.throttleThreshold
}









func (r *resultStore) AddFetch(header *types.Header, fastSync bool) (stale, throttled bool, item *fetchResult, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	var index int
	item, index, stale, throttled, err = r.getFetchResult(header.Number.Uint64())
	if err != nil || stale || throttled {
		return stale, throttled, item, err
	}
	if item == nil {
		item = newFetchResult(header, fastSync)
		r.items[index] = item
	}
	return stale, throttled, item, err
}





func (r *resultStore) GetDeliverySlot(headerNumber uint64) (*fetchResult, bool, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	res, _, stale, _, err := r.getFetchResult(headerNumber)
	return res, stale, err
}



func (r *resultStore) getFetchResult(headerNumber uint64) (item *fetchResult, index int, stale, throttle bool, err error) {
	index = int(int64(headerNumber) - int64(r.resultOffset))
	throttle = index >= int(r.throttleThreshold)
	stale = index < 0

	if index >= len(r.items) {
		err = fmt.Errorf("%w: index allocation went beyond available resultStore space "+
			"(index [%d] = header [%d] - resultOffset [%d], len(resultStore) = %d", errInvalidChain,
			index, headerNumber, r.resultOffset, len(r.items))
		return nil, index, stale, throttle, err
	}
	if stale {
		return nil, index, stale, throttle, nil
	}
	item = r.items[index]
	return item, index, stale, throttle, nil
}



func (r *resultStore) HasCompletedItems() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if len(r.items) == 0 {
		return false
	}
	if item := r.items[0]; item != nil && item.AllDone() {
		return true
	}
	return false
}





func (r *resultStore) countCompleted() int {
	
	
	index := atomic.LoadInt32(&r.indexIncomplete)
	for ; ; index++ {
		if index >= int32(len(r.items)) {
			break
		}
		result := r.items[index]
		if result == nil || !result.AllDone() {
			break
		}
	}
	atomic.StoreInt32(&r.indexIncomplete, index)
	return int(index)
}


func (r *resultStore) GetCompleted(limit int) []*fetchResult {
	r.lock.Lock()
	defer r.lock.Unlock()

	completed := r.countCompleted()
	if limit > completed {
		limit = completed
	}
	results := make([]*fetchResult, limit)
	copy(results, r.items[:limit])

	
	copy(r.items, r.items[limit:])
	for i := len(r.items) - limit; i < len(r.items); i++ {
		r.items[i] = nil
	}
	
	r.resultOffset += uint64(limit)
	atomic.AddInt32(&r.indexIncomplete, int32(-limit))

	return results
}


func (r *resultStore) Prepare(offset uint64) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.resultOffset < offset {
		r.resultOffset = offset
	}
}
