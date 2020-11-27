















package les

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/light"
)

var (
	retryQueue         = time.Millisecond * 100
	hardRequestTimeout = time.Second * 10
)



type retrieveManager struct {
	dist               *requestDistributor
	peers              *serverPeerSet
	softRequestTimeout func() time.Duration

	lock     sync.RWMutex
	sentReqs map[uint64]*sentReq
}


type validatorFunc func(distPeer, *Msg) error


type sentReq struct {
	rm       *retrieveManager
	req      *distReq
	id       uint64
	validate validatorFunc

	eventsCh chan reqPeerEvent
	stopCh   chan struct{}
	stopped  bool
	err      error

	lock   sync.RWMutex 
	sentTo map[distPeer]sentReqToPeer

	lastReqQueued bool     
	lastReqSentTo distPeer 
	reqSrtoCount  int      
}





type sentReqToPeer struct {
	delivered, frozen bool
	event             chan int
}



type reqPeerEvent struct {
	event int
	peer  distPeer
}

const (
	rpSent = iota 
	rpSoftTimeout
	rpHardTimeout
	rpDeliveredValid
	rpDeliveredInvalid
	rpNotDelivered
)


func newRetrieveManager(peers *serverPeerSet, dist *requestDistributor, srto func() time.Duration) *retrieveManager {
	return &retrieveManager{
		peers:              peers,
		dist:               dist,
		sentReqs:           make(map[uint64]*sentReq),
		softRequestTimeout: srto,
	}
}





func (rm *retrieveManager) retrieve(ctx context.Context, reqID uint64, req *distReq, val validatorFunc, shutdown chan struct{}) error {
	sentReq := rm.sendReq(reqID, req, val)
	select {
	case <-sentReq.stopCh:
	case <-ctx.Done():
		sentReq.stop(ctx.Err())
	case <-shutdown:
		sentReq.stop(fmt.Errorf("client is shutting down"))
	}
	return sentReq.getError()
}



func (rm *retrieveManager) sendReq(reqID uint64, req *distReq, val validatorFunc) *sentReq {
	r := &sentReq{
		rm:       rm,
		req:      req,
		id:       reqID,
		sentTo:   make(map[distPeer]sentReqToPeer),
		stopCh:   make(chan struct{}),
		eventsCh: make(chan reqPeerEvent, 10),
		validate: val,
	}

	canSend := req.canSend
	req.canSend = func(p distPeer) bool {
		
		r.lock.RLock()
		_, sent := r.sentTo[p]
		r.lock.RUnlock()
		return !sent && canSend(p)
	}

	request := req.request
	req.request = func(p distPeer) func() {
		
		r.lock.Lock()
		r.sentTo[p] = sentReqToPeer{delivered: false, frozen: false, event: make(chan int, 1)}
		r.lock.Unlock()
		return request(p)
	}
	rm.lock.Lock()
	rm.sentReqs[reqID] = r
	rm.lock.Unlock()

	go r.retrieveLoop()
	return r
}


func (rm *retrieveManager) deliver(peer distPeer, msg *Msg) error {
	rm.lock.RLock()
	req, ok := rm.sentReqs[msg.ReqID]
	rm.lock.RUnlock()

	if ok {
		return req.deliver(peer, msg)
	}
	return errResp(ErrUnexpectedResponse, "reqID = %v", msg.ReqID)
}



func (rm *retrieveManager) frozen(peer distPeer) {
	rm.lock.RLock()
	defer rm.lock.RUnlock()

	for _, req := range rm.sentReqs {
		req.frozen(peer)
	}
}


type reqStateFn func() reqStateFn


func (r *sentReq) retrieveLoop() {
	go r.tryRequest()
	r.lastReqQueued = true
	state := r.stateRequesting

	for state != nil {
		state = state()
	}

	r.rm.lock.Lock()
	delete(r.rm.sentReqs, r.id)
	r.rm.lock.Unlock()
}



func (r *sentReq) stateRequesting() reqStateFn {
	select {
	case ev := <-r.eventsCh:
		r.update(ev)
		switch ev.event {
		case rpSent:
			if ev.peer == nil {
				
				if r.waiting() {
					
					return r.stateNoMorePeers
				}
				
				r.stop(light.ErrNoPeers)
				
				return nil
			}
		case rpSoftTimeout:
			
			go r.tryRequest()
			r.lastReqQueued = true
			return r.stateRequesting
		case rpDeliveredInvalid, rpNotDelivered:
			
			if !r.lastReqQueued && r.lastReqSentTo == nil {
				go r.tryRequest()
				r.lastReqQueued = true
			}
			return r.stateRequesting
		case rpDeliveredValid:
			r.stop(nil)
			return r.stateStopped
		}
		return r.stateRequesting
	case <-r.stopCh:
		return r.stateStopped
	}
}




func (r *sentReq) stateNoMorePeers() reqStateFn {
	select {
	case <-time.After(retryQueue):
		go r.tryRequest()
		r.lastReqQueued = true
		return r.stateRequesting
	case ev := <-r.eventsCh:
		r.update(ev)
		if ev.event == rpDeliveredValid {
			r.stop(nil)
			return r.stateStopped
		}
		if r.waiting() {
			return r.stateNoMorePeers
		}
		r.stop(light.ErrNoPeers)
		return nil
	case <-r.stopCh:
		return r.stateStopped
	}
}



func (r *sentReq) stateStopped() reqStateFn {
	for r.waiting() {
		r.update(<-r.eventsCh)
	}
	return nil
}


func (r *sentReq) update(ev reqPeerEvent) {
	switch ev.event {
	case rpSent:
		r.lastReqQueued = false
		r.lastReqSentTo = ev.peer
	case rpSoftTimeout:
		r.lastReqSentTo = nil
		r.reqSrtoCount++
	case rpHardTimeout:
		r.reqSrtoCount--
	case rpDeliveredValid, rpDeliveredInvalid, rpNotDelivered:
		if ev.peer == r.lastReqSentTo {
			r.lastReqSentTo = nil
		} else {
			r.reqSrtoCount--
		}
	}
}



func (r *sentReq) waiting() bool {
	return r.lastReqQueued || r.lastReqSentTo != nil || r.reqSrtoCount > 0
}




func (r *sentReq) tryRequest() {
	sent := r.rm.dist.queue(r.req)
	var p distPeer
	select {
	case p = <-sent:
	case <-r.stopCh:
		if r.rm.dist.cancel(r.req) {
			p = nil
		} else {
			p = <-sent
		}
	}

	r.eventsCh <- reqPeerEvent{rpSent, p}
	if p == nil {
		return
	}

	hrto := false

	r.lock.RLock()
	s, ok := r.sentTo[p]
	r.lock.RUnlock()
	if !ok {
		panic(nil)
	}

	defer func() {
		
		pp, ok := p.(*serverPeer)
		if hrto && ok {
			pp.Log().Debug("Request timed out hard")
			if r.rm.peers != nil {
				r.rm.peers.unregister(pp.id)
			}
		}

		r.lock.Lock()
		delete(r.sentTo, p)
		r.lock.Unlock()
	}()

	select {
	case event := <-s.event:
		if event == rpNotDelivered {
			r.lock.Lock()
			delete(r.sentTo, p)
			r.lock.Unlock()
		}
		r.eventsCh <- reqPeerEvent{event, p}
		return
	case <-time.After(r.rm.softRequestTimeout()):
		r.eventsCh <- reqPeerEvent{rpSoftTimeout, p}
	}

	select {
	case event := <-s.event:
		if event == rpNotDelivered {
			r.lock.Lock()
			delete(r.sentTo, p)
			r.lock.Unlock()
		}
		r.eventsCh <- reqPeerEvent{event, p}
	case <-time.After(hardRequestTimeout):
		hrto = true
		r.eventsCh <- reqPeerEvent{rpHardTimeout, p}
	}
}


func (r *sentReq) deliver(peer distPeer, msg *Msg) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	s, ok := r.sentTo[peer]
	if !ok || s.delivered {
		return errResp(ErrUnexpectedResponse, "reqID = %v", msg.ReqID)
	}
	if s.frozen {
		return nil
	}
	valid := r.validate(peer, msg) == nil
	r.sentTo[peer] = sentReqToPeer{delivered: true, frozen: false, event: s.event}
	if valid {
		s.event <- rpDeliveredValid
	} else {
		s.event <- rpDeliveredInvalid
	}
	if !valid {
		return errResp(ErrInvalidResponse, "reqID = %v", msg.ReqID)
	}
	return nil
}





func (r *sentReq) frozen(peer distPeer) {
	r.lock.Lock()
	defer r.lock.Unlock()

	s, ok := r.sentTo[peer]
	if ok && !s.delivered && !s.frozen {
		r.sentTo[peer] = sentReqToPeer{delivered: false, frozen: true, event: s.event}
		s.event <- rpNotDelivered
	}
}



func (r *sentReq) stop(err error) {
	r.lock.Lock()
	if !r.stopped {
		r.stopped = true
		r.err = err
		close(r.stopCh)
	}
	r.lock.Unlock()
}



func (r *sentReq) getError() error {
	return r.err
}


func genReqID() uint64 {
	var rnd [8]byte
	rand.Read(rnd[:])
	return binary.BigEndian.Uint64(rnd[:])
}
