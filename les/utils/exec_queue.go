















package utils

import "sync"



type ExecQueue struct {
	mu        sync.Mutex
	cond      *sync.Cond
	funcs     []func()
	closeWait chan struct{}
}


func NewExecQueue(capacity int) *ExecQueue {
	q := &ExecQueue{funcs: make([]func(), 0, capacity)}
	q.cond = sync.NewCond(&q.mu)
	go q.loop()
	return q
}

func (q *ExecQueue) loop() {
	for f := q.waitNext(false); f != nil; f = q.waitNext(true) {
		f()
	}
	close(q.closeWait)
}

func (q *ExecQueue) waitNext(drop bool) (f func()) {
	q.mu.Lock()
	if drop && len(q.funcs) > 0 {
		
		
		q.funcs = append(q.funcs[:0], q.funcs[1:]...)
	}
	for !q.isClosed() {
		if len(q.funcs) > 0 {
			f = q.funcs[0]
			break
		}
		q.cond.Wait()
	}
	q.mu.Unlock()
	return f
}

func (q *ExecQueue) isClosed() bool {
	return q.closeWait != nil
}


func (q *ExecQueue) CanQueue() bool {
	q.mu.Lock()
	ok := !q.isClosed() && len(q.funcs) < cap(q.funcs)
	q.mu.Unlock()
	return ok
}


func (q *ExecQueue) Queue(f func()) bool {
	q.mu.Lock()
	ok := !q.isClosed() && len(q.funcs) < cap(q.funcs)
	if ok {
		q.funcs = append(q.funcs, f)
		q.cond.Signal()
	}
	q.mu.Unlock()
	return ok
}


func (q *ExecQueue) Clear() {
	q.mu.Lock()
	q.funcs = q.funcs[:0]
	q.mu.Unlock()
}




func (q *ExecQueue) Quit() {
	q.mu.Lock()
	if !q.isClosed() {
		q.closeWait = make(chan struct{})
		q.cond.Signal()
	}
	q.mu.Unlock()
	<-q.closeWait
}
