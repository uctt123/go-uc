















package prque

import (
	"container/heap"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
)









type LazyQueue struct {
	clock mclock.Clock
	
	
	
	queue                      [2]*sstack
	popQueue                   *sstack
	period                     time.Duration
	maxUntil                   mclock.AbsTime
	indexOffset                int
	setIndex                   SetIndexCallback
	priority                   PriorityCallback
	maxPriority                MaxPriorityCallback
	lastRefresh1, lastRefresh2 mclock.AbsTime
}

type (
	PriorityCallback    func(data interface{}, now mclock.AbsTime) int64   
	MaxPriorityCallback func(data interface{}, until mclock.AbsTime) int64 
)


func NewLazyQueue(setIndex SetIndexCallback, priority PriorityCallback, maxPriority MaxPriorityCallback, clock mclock.Clock, refreshPeriod time.Duration) *LazyQueue {
	q := &LazyQueue{
		popQueue:     newSstack(nil),
		setIndex:     setIndex,
		priority:     priority,
		maxPriority:  maxPriority,
		clock:        clock,
		period:       refreshPeriod,
		lastRefresh1: clock.Now(),
		lastRefresh2: clock.Now(),
	}
	q.Reset()
	q.refresh(clock.Now())
	return q
}


func (q *LazyQueue) Reset() {
	q.queue[0] = newSstack(q.setIndex0)
	q.queue[1] = newSstack(q.setIndex1)
}


func (q *LazyQueue) Refresh() {
	now := q.clock.Now()
	for time.Duration(now-q.lastRefresh2) >= q.period*2 {
		q.refresh(now)
		q.lastRefresh2 = q.lastRefresh1
		q.lastRefresh1 = now
	}
}


func (q *LazyQueue) refresh(now mclock.AbsTime) {
	q.maxUntil = now + mclock.AbsTime(q.period)
	for q.queue[0].Len() != 0 {
		q.Push(heap.Pop(q.queue[0]).(*item).value)
	}
	q.queue[0], q.queue[1] = q.queue[1], q.queue[0]
	q.indexOffset = 1 - q.indexOffset
	q.maxUntil += mclock.AbsTime(q.period)
}


func (q *LazyQueue) Push(data interface{}) {
	heap.Push(q.queue[1], &item{data, q.maxPriority(data, q.maxUntil)})
}


func (q *LazyQueue) Update(index int) {
	q.Push(q.Remove(index))
}


func (q *LazyQueue) Pop() (interface{}, int64) {
	var (
		resData interface{}
		resPri  int64
	)
	q.MultiPop(func(data interface{}, priority int64) bool {
		resData = data
		resPri = priority
		return false
	})
	return resData, resPri
}



func (q *LazyQueue) peekIndex() int {
	if q.queue[0].Len() != 0 {
		if q.queue[1].Len() != 0 && q.queue[1].blocks[0][0].priority > q.queue[0].blocks[0][0].priority {
			return 1
		}
		return 0
	}
	if q.queue[1].Len() != 0 {
		return 1
	}
	return -1
}




func (q *LazyQueue) MultiPop(callback func(data interface{}, priority int64) bool) {
	now := q.clock.Now()
	nextIndex := q.peekIndex()
	for nextIndex != -1 {
		data := heap.Pop(q.queue[nextIndex]).(*item).value
		heap.Push(q.popQueue, &item{data, q.priority(data, now)})
		nextIndex = q.peekIndex()
		for q.popQueue.Len() != 0 && (nextIndex == -1 || q.queue[nextIndex].blocks[0][0].priority < q.popQueue.blocks[0][0].priority) {
			i := heap.Pop(q.popQueue).(*item)
			if !callback(i.value, i.priority) {
				for q.popQueue.Len() != 0 {
					q.Push(heap.Pop(q.popQueue).(*item).value)
				}
				return
			}
			nextIndex = q.peekIndex() 
		}
	}
}


func (q *LazyQueue) PopItem() interface{} {
	i, _ := q.Pop()
	return i
}


func (q *LazyQueue) Remove(index int) interface{} {
	if index < 0 {
		return nil
	}
	return heap.Remove(q.queue[index&1^q.indexOffset], index>>1).(*item).value
}


func (q *LazyQueue) Empty() bool {
	return q.queue[0].Len() == 0 && q.queue[1].Len() == 0
}


func (q *LazyQueue) Size() int {
	return q.queue[0].Len() + q.queue[1].Len()
}


func (q *LazyQueue) setIndex0(data interface{}, index int) {
	if index == -1 {
		q.setIndex(data, -1)
	} else {
		q.setIndex(data, index+index)
	}
}


func (q *LazyQueue) setIndex1(data interface{}, index int) {
	q.setIndex(data, index+index+1)
}
