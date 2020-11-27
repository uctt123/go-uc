















package mclock

import (
	"container/heap"
	"sync"
	"time"
)










type Simulated struct {
	now       AbsTime
	scheduled simTimerHeap
	mu        sync.RWMutex
	cond      *sync.Cond
}


type simTimer struct {
	at    AbsTime
	index int 
	s     *Simulated
	do    func()
	ch    <-chan AbsTime
}

func (s *Simulated) init() {
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
}


func (s *Simulated) Run(d time.Duration) {
	s.mu.Lock()
	s.init()

	end := s.now + AbsTime(d)
	var do []func()
	for len(s.scheduled) > 0 && s.scheduled[0].at <= end {
		ev := heap.Pop(&s.scheduled).(*simTimer)
		do = append(do, ev.do)
	}
	s.now = end
	s.mu.Unlock()

	for _, fn := range do {
		fn()
	}
}


func (s *Simulated) ActiveTimers() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.scheduled)
}


func (s *Simulated) WaitForTimers(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()

	for len(s.scheduled) < n {
		s.cond.Wait()
	}
}


func (s *Simulated) Now() AbsTime {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.now
}


func (s *Simulated) Sleep(d time.Duration) {
	<-s.After(d)
}


func (s *Simulated) NewTimer(d time.Duration) ChanTimer {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan AbsTime, 1)
	var timer *simTimer
	timer = s.schedule(d, func() { ch <- timer.at })
	timer.ch = ch
	return timer
}



func (s *Simulated) After(d time.Duration) <-chan AbsTime {
	return s.NewTimer(d).C()
}



func (s *Simulated) AfterFunc(d time.Duration, fn func()) Timer {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.schedule(d, fn)
}

func (s *Simulated) schedule(d time.Duration, fn func()) *simTimer {
	s.init()

	at := s.now + AbsTime(d)
	ev := &simTimer{do: fn, at: at, s: s}
	heap.Push(&s.scheduled, ev)
	s.cond.Broadcast()
	return ev
}

func (ev *simTimer) Stop() bool {
	ev.s.mu.Lock()
	defer ev.s.mu.Unlock()

	if ev.index < 0 {
		return false
	}
	heap.Remove(&ev.s.scheduled, ev.index)
	ev.s.cond.Broadcast()
	ev.index = -1
	return true
}

func (ev *simTimer) Reset(d time.Duration) {
	if ev.ch == nil {
		panic("mclock: Reset() on timer created by AfterFunc")
	}

	ev.s.mu.Lock()
	defer ev.s.mu.Unlock()
	ev.at = ev.s.now.Add(d)
	if ev.index < 0 {
		heap.Push(&ev.s.scheduled, ev) 
	} else {
		heap.Fix(&ev.s.scheduled, ev.index) 
	}
	ev.s.cond.Broadcast()
}

func (ev *simTimer) C() <-chan AbsTime {
	if ev.ch == nil {
		panic("mclock: C() on timer created by AfterFunc")
	}
	return ev.ch
}

type simTimerHeap []*simTimer

func (h *simTimerHeap) Len() int {
	return len(*h)
}

func (h *simTimerHeap) Less(i, j int) bool {
	return (*h)[i].at < (*h)[j].at
}

func (h *simTimerHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
	(*h)[i].index = i
	(*h)[j].index = j
}

func (h *simTimerHeap) Push(x interface{}) {
	t := x.(*simTimer)
	t.index = len(*h)
	*h = append(*h, t)
}

func (h *simTimerHeap) Pop() interface{} {
	end := len(*h) - 1
	t := (*h)[end]
	t.index = -1
	(*h)[end] = nil
	*h = (*h)[:end]
	return t
}
