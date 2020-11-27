















package flowcontrol

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/common/prque"
)



type cmNodeFields struct {
	corrBufValue   int64 
	rcLastIntValue int64 
	rcFullIntValue int64 
	queueIndex     int   
}










const FixedPointMultiplier = 1000000

var (
	capacityDropFactor          = 0.1
	capacityRaiseTC             = 1 / (3 * float64(time.Hour)) 
	capacityRaiseThresholdRatio = 1.125                        
)






type ClientManager struct {
	clock     mclock.Clock
	lock      sync.Mutex
	enabledCh chan struct{}
	stop      chan chan struct{}

	curve                                      PieceWiseLinear
	sumRecharge, totalRecharge, totalConnected uint64
	logTotalCap, totalCapacity                 float64
	logTotalCapRaiseLimit                      float64
	minLogTotalCap, maxLogTotalCap             float64
	capacityRaiseThreshold                     uint64
	capLastUpdate                              mclock.AbsTime
	totalCapacityCh                            chan uint64

	
	
	rcLastUpdate   mclock.AbsTime 
	rcLastIntValue int64          
	
	
	
	rcQueue *prque.Prque
}
























func NewClientManager(curve PieceWiseLinear, clock mclock.Clock) *ClientManager {
	cm := &ClientManager{
		clock:         clock,
		rcQueue:       prque.New(func(a interface{}, i int) { a.(*ClientNode).queueIndex = i }),
		capLastUpdate: clock.Now(),
		stop:          make(chan chan struct{}),
	}
	if curve != nil {
		cm.SetRechargeCurve(curve)
	}
	go func() {
		
		for {
			select {
			case <-time.After(time.Minute):
				cm.lock.Lock()
				cm.updateTotalCapacity(cm.clock.Now(), true)
				cm.lock.Unlock()
			case stop := <-cm.stop:
				close(stop)
				return
			}
		}
	}()
	return cm
}


func (cm *ClientManager) Stop() {
	stop := make(chan struct{})
	cm.stop <- stop
	<-stop
}


func (cm *ClientManager) SetRechargeCurve(curve PieceWiseLinear) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	now := cm.clock.Now()
	cm.updateRecharge(now)
	cm.curve = curve
	if len(curve) > 0 {
		cm.totalRecharge = curve[len(curve)-1].Y
	} else {
		cm.totalRecharge = 0
	}
}





func (cm *ClientManager) SetCapacityLimits(min, max, raiseThreshold uint64) {
	if min < 1 {
		min = 1
	}
	cm.minLogTotalCap = math.Log(float64(min))
	if max < 1 {
		max = 1
	}
	cm.maxLogTotalCap = math.Log(float64(max))
	cm.logTotalCap = cm.maxLogTotalCap
	cm.capacityRaiseThreshold = raiseThreshold
	cm.refreshCapacity()
}



func (cm *ClientManager) connect(node *ClientNode) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	now := cm.clock.Now()
	cm.updateRecharge(now)
	node.corrBufValue = int64(node.params.BufLimit)
	node.rcLastIntValue = cm.rcLastIntValue
	node.queueIndex = -1
	cm.updateTotalCapacity(now, true)
	cm.totalConnected += node.params.MinRecharge
	cm.updateRaiseLimit()
}


func (cm *ClientManager) disconnect(node *ClientNode) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	now := cm.clock.Now()
	cm.updateRecharge(cm.clock.Now())
	cm.updateTotalCapacity(now, true)
	cm.totalConnected -= node.params.MinRecharge
	cm.updateRaiseLimit()
}





func (cm *ClientManager) accepted(node *ClientNode, maxCost uint64, now mclock.AbsTime) (priority int64) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.updateNodeRc(node, -int64(maxCost), &node.params, now)
	rcTime := (node.params.BufLimit - uint64(node.corrBufValue)) * FixedPointMultiplier / node.params.MinRecharge
	return -int64(now) - int64(rcTime)
}





func (cm *ClientManager) processed(node *ClientNode, maxCost, realCost uint64, now mclock.AbsTime) {
	if realCost > maxCost {
		realCost = maxCost
	}
	cm.updateBuffer(node, int64(maxCost-realCost), now)
}



func (cm *ClientManager) updateBuffer(node *ClientNode, add int64, now mclock.AbsTime) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.updateNodeRc(node, add, &node.params, now)
	if node.corrBufValue > node.bufValue {
		if node.log != nil {
			node.log.add(now, fmt.Sprintf("corrected  bv=%d  oldBv=%d", node.corrBufValue, node.bufValue))
		}
		node.bufValue = node.corrBufValue
	}
}


func (cm *ClientManager) updateParams(node *ClientNode, params ServerParams, now mclock.AbsTime) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.updateRecharge(now)
	cm.updateTotalCapacity(now, true)
	cm.totalConnected += params.MinRecharge - node.params.MinRecharge
	cm.updateRaiseLimit()
	cm.updateNodeRc(node, 0, &params, now)
}



func (cm *ClientManager) updateRaiseLimit() {
	if cm.capacityRaiseThreshold == 0 {
		cm.logTotalCapRaiseLimit = 0
		return
	}
	limit := float64(cm.totalConnected + cm.capacityRaiseThreshold)
	limit2 := float64(cm.totalConnected) * capacityRaiseThresholdRatio
	if limit2 > limit {
		limit = limit2
	}
	if limit < 1 {
		limit = 1
	}
	cm.logTotalCapRaiseLimit = math.Log(limit)
}



func (cm *ClientManager) updateRecharge(now mclock.AbsTime) {
	lastUpdate := cm.rcLastUpdate
	cm.rcLastUpdate = now
	
	
	for cm.sumRecharge > 0 {
		sumRecharge := cm.sumRecharge
		if sumRecharge > cm.totalRecharge {
			sumRecharge = cm.totalRecharge
		}
		bonusRatio := float64(1)
		v := cm.curve.ValueAt(sumRecharge)
		s := float64(sumRecharge)
		if v > s && s > 0 {
			bonusRatio = v / s
		}
		dt := now - lastUpdate
		
		rcqNode := cm.rcQueue.PopItem().(*ClientNode) 
		
		dtNext := mclock.AbsTime(float64(rcqNode.rcFullIntValue-cm.rcLastIntValue) / bonusRatio)
		if dt < dtNext {
			
			
			cm.rcQueue.Push(rcqNode, -rcqNode.rcFullIntValue)
			cm.rcLastIntValue += int64(bonusRatio * float64(dt))
			return
		}
		lastUpdate += dtNext
		
		if rcqNode.corrBufValue < int64(rcqNode.params.BufLimit) {
			rcqNode.corrBufValue = int64(rcqNode.params.BufLimit)
			cm.sumRecharge -= rcqNode.params.MinRecharge
		}
		cm.rcLastIntValue = rcqNode.rcFullIntValue
	}
}



func (cm *ClientManager) updateNodeRc(node *ClientNode, bvc int64, params *ServerParams, now mclock.AbsTime) {
	cm.updateRecharge(now)
	wasFull := true
	if node.corrBufValue != int64(node.params.BufLimit) {
		wasFull = false
		node.corrBufValue += (cm.rcLastIntValue - node.rcLastIntValue) * int64(node.params.MinRecharge) / FixedPointMultiplier
		if node.corrBufValue > int64(node.params.BufLimit) {
			node.corrBufValue = int64(node.params.BufLimit)
		}
		node.rcLastIntValue = cm.rcLastIntValue
	}
	node.corrBufValue += bvc
	diff := int64(params.BufLimit - node.params.BufLimit)
	if diff > 0 {
		node.corrBufValue += diff
	}
	isFull := false
	if node.corrBufValue >= int64(params.BufLimit) {
		node.corrBufValue = int64(params.BufLimit)
		isFull = true
	}
	if !wasFull {
		cm.sumRecharge -= node.params.MinRecharge
	}
	if params != &node.params {
		node.params = *params
	}
	if !isFull {
		cm.sumRecharge += node.params.MinRecharge
		if node.queueIndex != -1 {
			cm.rcQueue.Remove(node.queueIndex)
		}
		node.rcLastIntValue = cm.rcLastIntValue
		node.rcFullIntValue = cm.rcLastIntValue + (int64(node.params.BufLimit)-node.corrBufValue)*FixedPointMultiplier/int64(node.params.MinRecharge)
		cm.rcQueue.Push(node, -node.rcFullIntValue)
	}
}


func (cm *ClientManager) reduceTotalCapacity(frozenCap uint64) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	ratio := float64(1)
	if frozenCap < cm.totalConnected {
		ratio = float64(frozenCap) / float64(cm.totalConnected)
	}
	now := cm.clock.Now()
	cm.updateTotalCapacity(now, false)
	cm.logTotalCap -= capacityDropFactor * ratio
	if cm.logTotalCap < cm.minLogTotalCap {
		cm.logTotalCap = cm.minLogTotalCap
	}
	cm.updateTotalCapacity(now, true)
}







func (cm *ClientManager) updateTotalCapacity(now mclock.AbsTime, refresh bool) {
	dt := now - cm.capLastUpdate
	cm.capLastUpdate = now

	if cm.logTotalCap < cm.logTotalCapRaiseLimit {
		cm.logTotalCap += capacityRaiseTC * float64(dt)
		if cm.logTotalCap > cm.logTotalCapRaiseLimit {
			cm.logTotalCap = cm.logTotalCapRaiseLimit
		}
	}
	if cm.logTotalCap > cm.maxLogTotalCap {
		cm.logTotalCap = cm.maxLogTotalCap
	}
	if refresh {
		cm.refreshCapacity()
	}
}



func (cm *ClientManager) refreshCapacity() {
	totalCapacity := math.Exp(cm.logTotalCap)
	if totalCapacity >= cm.totalCapacity*0.999 && totalCapacity <= cm.totalCapacity*1.001 {
		return
	}
	cm.totalCapacity = totalCapacity
	if cm.totalCapacityCh != nil {
		select {
		case cm.totalCapacityCh <- uint64(cm.totalCapacity):
		default:
		}
	}
}



func (cm *ClientManager) SubscribeTotalCapacity(ch chan uint64) uint64 {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.totalCapacityCh = ch
	return uint64(cm.totalCapacity)
}


type PieceWiseLinear []struct{ X, Y uint64 }


func (pwl PieceWiseLinear) ValueAt(x uint64) float64 {
	l := 0
	h := len(pwl)
	if h == 0 {
		return 0
	}
	for h != l {
		m := (l + h) / 2
		if x > pwl[m].X {
			l = m + 1
		} else {
			h = m
		}
	}
	if l == 0 {
		return float64(pwl[0].Y)
	}
	l--
	if h == len(pwl) {
		return float64(pwl[l].Y)
	}
	dx := pwl[h].X - pwl[l].X
	if dx < 1 {
		return float64(pwl[l].Y)
	}
	return float64(pwl[l].Y) + float64(pwl[h].Y-pwl[l].Y)*float64(x-pwl[l].X)/float64(dx)
}


func (pwl PieceWiseLinear) Valid() bool {
	var lastX uint64
	for _, i := range pwl {
		if i.X < lastX {
			return false
		}
		lastX = i.X
	}
	return true
}
