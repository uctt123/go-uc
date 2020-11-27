
















package flowcontrol

import (
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/log"
)

const (
	
	
	fcTimeConst = time.Millisecond
	
	
	
	DecParamDelay = time.Second * 2
	
	keepLogs = 0
)





type ServerParams struct {
	BufLimit, MinRecharge uint64
}


type scheduledUpdate struct {
	time   mclock.AbsTime
	params ServerParams
}



type ClientNode struct {
	params         ServerParams
	bufValue       int64
	lastTime       mclock.AbsTime
	updateSchedule []scheduledUpdate
	sumCost        uint64            
	accepted       map[uint64]uint64 
	connected      bool
	lock           sync.Mutex
	cm             *ClientManager
	log            *logger
	cmNodeFields
}


func NewClientNode(cm *ClientManager, params ServerParams) *ClientNode {
	node := &ClientNode{
		cm:        cm,
		params:    params,
		bufValue:  int64(params.BufLimit),
		lastTime:  cm.clock.Now(),
		accepted:  make(map[uint64]uint64),
		connected: true,
	}
	if keepLogs > 0 {
		node.log = newLogger(keepLogs)
	}
	cm.connect(node)
	return node
}


func (node *ClientNode) Disconnect() {
	node.lock.Lock()
	defer node.lock.Unlock()

	node.connected = false
	node.cm.disconnect(node)
}


func (node *ClientNode) BufferStatus() (uint64, uint64) {
	node.lock.Lock()
	defer node.lock.Unlock()

	if !node.connected {
		return 0, 0
	}
	now := node.cm.clock.Now()
	node.update(now)
	node.cm.updateBuffer(node, 0, now)
	bv := node.bufValue
	if bv < 0 {
		bv = 0
	}
	return uint64(bv), node.params.BufLimit
}






func (node *ClientNode) OneTimeCost(cost uint64) {
	node.lock.Lock()
	defer node.lock.Unlock()

	now := node.cm.clock.Now()
	node.update(now)
	node.bufValue -= int64(cost)
	node.cm.updateBuffer(node, -int64(cost), now)
}



func (node *ClientNode) Freeze() {
	node.lock.Lock()
	frozenCap := node.params.MinRecharge
	node.lock.Unlock()
	node.cm.reduceTotalCapacity(frozenCap)
}



func (node *ClientNode) update(now mclock.AbsTime) {
	for len(node.updateSchedule) > 0 && node.updateSchedule[0].time <= now {
		node.recalcBV(node.updateSchedule[0].time)
		node.updateParams(node.updateSchedule[0].params, now)
		node.updateSchedule = node.updateSchedule[1:]
	}
	node.recalcBV(now)
}


func (node *ClientNode) recalcBV(now mclock.AbsTime) {
	dt := uint64(now - node.lastTime)
	if now < node.lastTime {
		dt = 0
	}
	node.bufValue += int64(node.params.MinRecharge * dt / uint64(fcTimeConst))
	if node.bufValue > int64(node.params.BufLimit) {
		node.bufValue = int64(node.params.BufLimit)
	}
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("updated  bv=%d  MRR=%d  BufLimit=%d", node.bufValue, node.params.MinRecharge, node.params.BufLimit))
	}
	node.lastTime = now
}


func (node *ClientNode) UpdateParams(params ServerParams) {
	node.lock.Lock()
	defer node.lock.Unlock()

	now := node.cm.clock.Now()
	node.update(now)
	if params.MinRecharge >= node.params.MinRecharge {
		node.updateSchedule = nil
		node.updateParams(params, now)
	} else {
		for i, s := range node.updateSchedule {
			if params.MinRecharge >= s.params.MinRecharge {
				s.params = params
				node.updateSchedule = node.updateSchedule[:i+1]
				return
			}
		}
		node.updateSchedule = append(node.updateSchedule, scheduledUpdate{time: now + mclock.AbsTime(DecParamDelay), params: params})
	}
}


func (node *ClientNode) updateParams(params ServerParams, now mclock.AbsTime) {
	diff := int64(params.BufLimit - node.params.BufLimit)
	if diff > 0 {
		node.bufValue += diff
	} else if node.bufValue > int64(params.BufLimit) {
		node.bufValue = int64(params.BufLimit)
	}
	node.cm.updateParams(node, params, now)
}




func (node *ClientNode) AcceptRequest(reqID, index, maxCost uint64) (accepted bool, bufShort uint64, priority int64) {
	node.lock.Lock()
	defer node.lock.Unlock()

	now := node.cm.clock.Now()
	node.update(now)
	if int64(maxCost) > node.bufValue {
		if node.log != nil {
			node.log.add(now, fmt.Sprintf("rejected  reqID=%d  bv=%d  maxCost=%d", reqID, node.bufValue, maxCost))
			node.log.dump(now)
		}
		return false, maxCost - uint64(node.bufValue), 0
	}
	node.bufValue -= int64(maxCost)
	node.sumCost += maxCost
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("accepted  reqID=%d  bv=%d  maxCost=%d  sumCost=%d", reqID, node.bufValue, maxCost, node.sumCost))
	}
	node.accepted[index] = node.sumCost
	return true, 0, node.cm.accepted(node, maxCost, now)
}


func (node *ClientNode) RequestProcessed(reqID, index, maxCost, realCost uint64) uint64 {
	node.lock.Lock()
	defer node.lock.Unlock()

	now := node.cm.clock.Now()
	node.update(now)
	node.cm.processed(node, maxCost, realCost, now)
	bv := node.bufValue + int64(node.sumCost-node.accepted[index])
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("processed  reqID=%d  bv=%d  maxCost=%d  realCost=%d  sumCost=%d  oldSumCost=%d  reportedBV=%d", reqID, node.bufValue, maxCost, realCost, node.sumCost, node.accepted[index], bv))
	}
	delete(node.accepted, index)
	if bv < 0 {
		return 0
	}
	return uint64(bv)
}



type ServerNode struct {
	clock       mclock.Clock
	bufEstimate uint64
	bufRecharge bool
	lastTime    mclock.AbsTime
	params      ServerParams
	sumCost     uint64            
	pending     map[uint64]uint64 
	log         *logger
	lock        sync.RWMutex
}


func NewServerNode(params ServerParams, clock mclock.Clock) *ServerNode {
	node := &ServerNode{
		clock:       clock,
		bufEstimate: params.BufLimit,
		bufRecharge: false,
		lastTime:    clock.Now(),
		params:      params,
		pending:     make(map[uint64]uint64),
	}
	if keepLogs > 0 {
		node.log = newLogger(keepLogs)
	}
	return node
}


func (node *ServerNode) UpdateParams(params ServerParams) {
	node.lock.Lock()
	defer node.lock.Unlock()

	node.recalcBLE(mclock.Now())
	if params.BufLimit > node.params.BufLimit {
		node.bufEstimate += params.BufLimit - node.params.BufLimit
	} else {
		if node.bufEstimate > params.BufLimit {
			node.bufEstimate = params.BufLimit
		}
	}
	node.params = params
}



func (node *ServerNode) recalcBLE(now mclock.AbsTime) {
	if now < node.lastTime {
		return
	}
	if node.bufRecharge {
		dt := uint64(now - node.lastTime)
		node.bufEstimate += node.params.MinRecharge * dt / uint64(fcTimeConst)
		if node.bufEstimate >= node.params.BufLimit {
			node.bufEstimate = node.params.BufLimit
			node.bufRecharge = false
		}
	}
	node.lastTime = now
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("updated  bufEst=%d  MRR=%d  BufLimit=%d", node.bufEstimate, node.params.MinRecharge, node.params.BufLimit))
	}
}


const safetyMargin = time.Millisecond




func (node *ServerNode) CanSend(maxCost uint64) (time.Duration, float64) {
	node.lock.RLock()
	defer node.lock.RUnlock()

	now := node.clock.Now()
	node.recalcBLE(now)
	maxCost += uint64(safetyMargin) * node.params.MinRecharge / uint64(fcTimeConst)
	if maxCost > node.params.BufLimit {
		maxCost = node.params.BufLimit
	}
	if node.bufEstimate >= maxCost {
		relBuf := float64(node.bufEstimate-maxCost) / float64(node.params.BufLimit)
		if node.log != nil {
			node.log.add(now, fmt.Sprintf("canSend  bufEst=%d  maxCost=%d  true  relBuf=%f", node.bufEstimate, maxCost, relBuf))
		}
		return 0, relBuf
	}
	timeLeft := time.Duration((maxCost - node.bufEstimate) * uint64(fcTimeConst) / node.params.MinRecharge)
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("canSend  bufEst=%d  maxCost=%d  false  timeLeft=%v", node.bufEstimate, maxCost, timeLeft))
	}
	return timeLeft, 0
}




func (node *ServerNode) QueuedRequest(reqID, maxCost uint64) {
	node.lock.Lock()
	defer node.lock.Unlock()

	now := node.clock.Now()
	node.recalcBLE(now)
	
	
	
	if node.bufEstimate >= maxCost {
		node.bufEstimate -= maxCost
	} else {
		log.Error("Queued request with insufficient buffer estimate")
		node.bufEstimate = 0
	}
	node.sumCost += maxCost
	node.pending[reqID] = node.sumCost
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("queued  reqID=%d  bufEst=%d  maxCost=%d  sumCost=%d", reqID, node.bufEstimate, maxCost, node.sumCost))
	}
}



func (node *ServerNode) ReceivedReply(reqID, bv uint64) {
	node.lock.Lock()
	defer node.lock.Unlock()

	now := node.clock.Now()
	node.recalcBLE(now)
	if bv > node.params.BufLimit {
		bv = node.params.BufLimit
	}
	sc, ok := node.pending[reqID]
	if !ok {
		return
	}
	delete(node.pending, reqID)
	cc := node.sumCost - sc
	newEstimate := uint64(0)
	if bv > cc {
		newEstimate = bv - cc
	}
	if newEstimate > node.bufEstimate {
		
		
		
		node.bufEstimate = newEstimate
	}

	node.bufRecharge = node.bufEstimate < node.params.BufLimit
	node.lastTime = now
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("received  reqID=%d  bufEst=%d  reportedBv=%d  sumCost=%d  oldSumCost=%d", reqID, node.bufEstimate, bv, node.sumCost, sc))
	}
}



func (node *ServerNode) ResumeFreeze(bv uint64) {
	node.lock.Lock()
	defer node.lock.Unlock()

	for reqID := range node.pending {
		delete(node.pending, reqID)
	}
	now := node.clock.Now()
	node.recalcBLE(now)
	if bv > node.params.BufLimit {
		bv = node.params.BufLimit
	}
	node.bufEstimate = bv
	node.bufRecharge = node.bufEstimate < node.params.BufLimit
	node.lastTime = now
	if node.log != nil {
		node.log.add(now, fmt.Sprintf("unfreeze  bv=%d  sumCost=%d", bv, node.sumCost))
	}
}


func (node *ServerNode) DumpLogs() {
	node.lock.Lock()
	defer node.lock.Unlock()

	if node.log != nil {
		node.log.dump(node.clock.Now())
	}
}
