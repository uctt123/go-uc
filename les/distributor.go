















package les

import (
	"container/list"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/les/utils"
)




type requestDistributor struct {
	clock        mclock.Clock
	reqQueue     *list.List
	lastReqOrder uint64
	peers        map[distPeer]struct{}
	peerLock     sync.RWMutex
	loopChn      chan struct{}
	loopNextSent bool
	lock         sync.Mutex

	closeCh chan struct{}
	wg      sync.WaitGroup
}






type distPeer interface {
	waitBefore(uint64) (time.Duration, float64)
	canQueue() bool
	queueSend(f func()) bool
}









type distReq struct {
	getCost func(distPeer) uint64
	canSend func(distPeer) bool
	request func(distPeer) func()

	reqOrder     uint64
	sentChn      chan distPeer
	element      *list.Element
	waitForPeers mclock.AbsTime
	enterQueue   mclock.AbsTime
}


func newRequestDistributor(peers *serverPeerSet, clock mclock.Clock) *requestDistributor {
	d := &requestDistributor{
		clock:    clock,
		reqQueue: list.New(),
		loopChn:  make(chan struct{}, 2),
		closeCh:  make(chan struct{}),
		peers:    make(map[distPeer]struct{}),
	}
	if peers != nil {
		peers.subscribe(d)
	}
	d.wg.Add(1)
	go d.loop()
	return d
}


func (d *requestDistributor) registerPeer(p *serverPeer) {
	d.peerLock.Lock()
	d.peers[p] = struct{}{}
	d.peerLock.Unlock()
}


func (d *requestDistributor) unregisterPeer(p *serverPeer) {
	d.peerLock.Lock()
	delete(d.peers, p)
	d.peerLock.Unlock()
}


func (d *requestDistributor) registerTestPeer(p distPeer) {
	d.peerLock.Lock()
	d.peers[p] = struct{}{}
	d.peerLock.Unlock()
}

var (
	
	
	distMaxWait = time.Millisecond * 50

	
	
	waitForPeers = time.Second * 3
)


func (d *requestDistributor) loop() {
	defer d.wg.Done()
	for {
		select {
		case <-d.closeCh:
			d.lock.Lock()
			elem := d.reqQueue.Front()
			for elem != nil {
				req := elem.Value.(*distReq)
				close(req.sentChn)
				req.sentChn = nil
				elem = elem.Next()
			}
			d.lock.Unlock()
			return
		case <-d.loopChn:
			d.lock.Lock()
			d.loopNextSent = false
		loop:
			for {
				peer, req, wait := d.nextRequest()
				if req != nil && wait == 0 {
					chn := req.sentChn 
					d.remove(req)
					send := req.request(peer)
					if send != nil {
						peer.queueSend(send)
						requestSendDelay.Update(time.Duration(d.clock.Now() - req.enterQueue))
					}
					chn <- peer
					close(chn)
				} else {
					if wait == 0 {
						
						
						break loop
					}
					d.loopNextSent = true 
					if wait > distMaxWait {
						
						wait = distMaxWait
					}
					go func() {
						d.clock.Sleep(wait)
						d.loopChn <- struct{}{}
					}()
					break loop
				}
			}
			d.lock.Unlock()
		}
	}
}


type selectPeerItem struct {
	peer   distPeer
	req    *distReq
	weight uint64
}

func selectPeerWeight(i interface{}) uint64 {
	return i.(selectPeerItem).weight
}



func (d *requestDistributor) nextRequest() (distPeer, *distReq, time.Duration) {
	checkedPeers := make(map[distPeer]struct{})
	elem := d.reqQueue.Front()
	var (
		bestWait time.Duration
		sel      *utils.WeightedRandomSelect
	)

	d.peerLock.RLock()
	defer d.peerLock.RUnlock()

	peerCount := len(d.peers)
	for (len(checkedPeers) < peerCount || elem == d.reqQueue.Front()) && elem != nil {
		req := elem.Value.(*distReq)
		canSend := false
		now := d.clock.Now()
		if req.waitForPeers > now {
			canSend = true
			wait := time.Duration(req.waitForPeers - now)
			if bestWait == 0 || wait < bestWait {
				bestWait = wait
			}
		}
		for peer := range d.peers {
			if _, ok := checkedPeers[peer]; !ok && peer.canQueue() && req.canSend(peer) {
				canSend = true
				cost := req.getCost(peer)
				wait, bufRemain := peer.waitBefore(cost)
				if wait == 0 {
					if sel == nil {
						sel = utils.NewWeightedRandomSelect(selectPeerWeight)
					}
					sel.Update(selectPeerItem{peer: peer, req: req, weight: uint64(bufRemain*1000000) + 1})
				} else {
					if bestWait == 0 || wait < bestWait {
						bestWait = wait
					}
				}
				checkedPeers[peer] = struct{}{}
			}
		}
		next := elem.Next()
		if !canSend && elem == d.reqQueue.Front() {
			close(req.sentChn)
			d.remove(req)
		}
		elem = next
	}

	if sel != nil {
		c := sel.Choose().(selectPeerItem)
		return c.peer, c.req, 0
	}
	return nil, nil, bestWait
}





func (d *requestDistributor) queue(r *distReq) chan distPeer {
	d.lock.Lock()
	defer d.lock.Unlock()

	if r.reqOrder == 0 {
		d.lastReqOrder++
		r.reqOrder = d.lastReqOrder
		r.waitForPeers = d.clock.Now() + mclock.AbsTime(waitForPeers)
	}
	
	
	r.enterQueue = d.clock.Now()

	back := d.reqQueue.Back()
	if back == nil || r.reqOrder > back.Value.(*distReq).reqOrder {
		r.element = d.reqQueue.PushBack(r)
	} else {
		before := d.reqQueue.Front()
		for before.Value.(*distReq).reqOrder < r.reqOrder {
			before = before.Next()
		}
		r.element = d.reqQueue.InsertBefore(r, before)
	}

	if !d.loopNextSent {
		d.loopNextSent = true
		d.loopChn <- struct{}{}
	}

	r.sentChn = make(chan distPeer, 1)
	return r.sentChn
}




func (d *requestDistributor) cancel(r *distReq) bool {
	d.lock.Lock()
	defer d.lock.Unlock()

	if r.sentChn == nil {
		return false
	}

	close(r.sentChn)
	d.remove(r)
	return true
}


func (d *requestDistributor) remove(r *distReq) {
	r.sentChn = nil
	if r.element != nil {
		d.reqQueue.Remove(r.element)
		r.element = nil
	}
}

func (d *requestDistributor) close() {
	close(d.closeCh)
	d.wg.Wait()
}
