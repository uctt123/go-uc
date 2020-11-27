















package server

import (
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nodestate"
)

const (
	lazyQueueRefresh = time.Second * 10 
)




type PriorityPoolSetup struct {
	
	ActiveFlag, InactiveFlag       nodestate.Flags
	CapacityField, ppNodeInfoField nodestate.Field
	
	updateFlag    nodestate.Flags
	priorityField nodestate.Field
}



func NewPriorityPoolSetup(setup *nodestate.Setup) PriorityPoolSetup {
	return PriorityPoolSetup{
		ActiveFlag:      setup.NewFlag("active"),
		InactiveFlag:    setup.NewFlag("inactive"),
		CapacityField:   setup.NewField("capacity", reflect.TypeOf(uint64(0))),
		ppNodeInfoField: setup.NewField("ppNodeInfo", reflect.TypeOf(&ppNodeInfo{})),
	}
}


func (pps *PriorityPoolSetup) Connect(priorityField nodestate.Field, updateFlag nodestate.Flags) {
	pps.priorityField = priorityField 
	pps.updateFlag = updateFlag       
}



























type PriorityPool struct {
	PriorityPoolSetup
	ns                     *nodestate.NodeStateMachine
	clock                  mclock.Clock
	lock                   sync.Mutex
	activeQueue            *prque.LazyQueue
	inactiveQueue          *prque.Prque
	changed                []*ppNodeInfo
	activeCount, activeCap uint64
	maxCount, maxCap       uint64
	minCap                 uint64
	activeBias             time.Duration
	capacityStepDiv        uint64
}


type nodePriority interface {
	
	Priority(now mclock.AbsTime, cap uint64) int64
	
	
	
	
	EstMinPriority(until mclock.AbsTime, cap uint64, update bool) int64
}


type ppNodeInfo struct {
	nodePriority               nodePriority
	node                       *enode.Node
	connected                  bool
	capacity, origCap          uint64
	bias                       time.Duration
	forced, changed            bool
	activeIndex, inactiveIndex int
}


func NewPriorityPool(ns *nodestate.NodeStateMachine, setup PriorityPoolSetup, clock mclock.Clock, minCap uint64, activeBias time.Duration, capacityStepDiv uint64) *PriorityPool {
	pp := &PriorityPool{
		ns:                ns,
		PriorityPoolSetup: setup,
		clock:             clock,
		activeQueue:       prque.NewLazyQueue(activeSetIndex, activePriority, activeMaxPriority, clock, lazyQueueRefresh),
		inactiveQueue:     prque.New(inactiveSetIndex),
		minCap:            minCap,
		activeBias:        activeBias,
		capacityStepDiv:   capacityStepDiv,
	}

	ns.SubscribeField(pp.priorityField, func(node *enode.Node, state nodestate.Flags, oldValue, newValue interface{}) {
		if newValue != nil {
			c := &ppNodeInfo{
				node:          node,
				nodePriority:  newValue.(nodePriority),
				activeIndex:   -1,
				inactiveIndex: -1,
			}
			ns.SetFieldSub(node, pp.ppNodeInfoField, c)
		} else {
			ns.SetStateSub(node, nodestate.Flags{}, pp.ActiveFlag.Or(pp.InactiveFlag), 0)
			if n, _ := pp.ns.GetField(node, pp.ppNodeInfoField).(*ppNodeInfo); n != nil {
				pp.disconnectedNode(n)
			}
			ns.SetFieldSub(node, pp.CapacityField, nil)
			ns.SetFieldSub(node, pp.ppNodeInfoField, nil)
		}
	})
	ns.SubscribeState(pp.ActiveFlag.Or(pp.InactiveFlag), func(node *enode.Node, oldState, newState nodestate.Flags) {
		if c, _ := pp.ns.GetField(node, pp.ppNodeInfoField).(*ppNodeInfo); c != nil {
			if oldState.IsEmpty() {
				pp.connectedNode(c)
			}
			if newState.IsEmpty() {
				pp.disconnectedNode(c)
			}
		}
	})
	ns.SubscribeState(pp.updateFlag, func(node *enode.Node, oldState, newState nodestate.Flags) {
		if !newState.IsEmpty() {
			pp.updatePriority(node)
		}
	})
	return pp
}












func (pp *PriorityPool) RequestCapacity(node *enode.Node, targetCap uint64, bias time.Duration, setCap bool) (minPriority int64, allowed bool) {
	pp.lock.Lock()
	pp.activeQueue.Refresh()
	var updates []capUpdate
	defer func() {
		pp.lock.Unlock()
		pp.updateFlags(updates)
	}()

	if targetCap < pp.minCap {
		targetCap = pp.minCap
	}
	c, _ := pp.ns.GetField(node, pp.ppNodeInfoField).(*ppNodeInfo)
	if c == nil {
		log.Error("RequestCapacity called for unknown node", "id", node.ID())
		return math.MaxInt64, false
	}
	var priority int64
	if targetCap > c.capacity {
		priority = c.nodePriority.EstMinPriority(pp.clock.Now()+mclock.AbsTime(bias), targetCap, false)
	} else {
		priority = c.nodePriority.Priority(pp.clock.Now(), targetCap)
	}
	pp.markForChange(c)
	pp.setCapacity(c, targetCap)
	c.forced = true
	pp.activeQueue.Remove(c.activeIndex)
	pp.inactiveQueue.Remove(c.inactiveIndex)
	pp.activeQueue.Push(c)
	minPriority = pp.enforceLimits()
	
	
	allowed = priority > minPriority
	updates = pp.finalizeChanges(setCap && allowed)
	return
}


func (pp *PriorityPool) SetLimits(maxCount, maxCap uint64) {
	pp.lock.Lock()
	pp.activeQueue.Refresh()
	var updates []capUpdate
	defer func() {
		pp.lock.Unlock()
		pp.ns.Operation(func() { pp.updateFlags(updates) })
	}()

	inc := (maxCount > pp.maxCount) || (maxCap > pp.maxCap)
	dec := (maxCount < pp.maxCount) || (maxCap < pp.maxCap)
	pp.maxCount, pp.maxCap = maxCount, maxCap
	if dec {
		pp.enforceLimits()
		updates = pp.finalizeChanges(true)
	}
	if inc {
		updates = pp.tryActivate()
	}
}


func (pp *PriorityPool) SetActiveBias(bias time.Duration) {
	pp.lock.Lock()
	defer pp.lock.Unlock()

	pp.activeBias = bias
	pp.tryActivate()
}


func (pp *PriorityPool) ActiveCapacity() uint64 {
	pp.lock.Lock()
	defer pp.lock.Unlock()

	return pp.activeCap
}


func inactiveSetIndex(a interface{}, index int) {
	a.(*ppNodeInfo).inactiveIndex = index
}


func activeSetIndex(a interface{}, index int) {
	a.(*ppNodeInfo).activeIndex = index
}



func invertPriority(p int64) int64 {
	if p == math.MinInt64 {
		return math.MaxInt64
	}
	return -p
}


func activePriority(a interface{}, now mclock.AbsTime) int64 {
	c := a.(*ppNodeInfo)
	if c.forced {
		return math.MinInt64
	}
	if c.bias == 0 {
		return invertPriority(c.nodePriority.Priority(now, c.capacity))
	} else {
		return invertPriority(c.nodePriority.EstMinPriority(now+mclock.AbsTime(c.bias), c.capacity, true))
	}
}


func activeMaxPriority(a interface{}, until mclock.AbsTime) int64 {
	c := a.(*ppNodeInfo)
	if c.forced {
		return math.MinInt64
	}
	return invertPriority(c.nodePriority.EstMinPriority(until+mclock.AbsTime(c.bias), c.capacity, false))
}


func (pp *PriorityPool) inactivePriority(p *ppNodeInfo) int64 {
	return p.nodePriority.Priority(pp.clock.Now(), pp.minCap)
}



func (pp *PriorityPool) connectedNode(c *ppNodeInfo) {
	pp.lock.Lock()
	pp.activeQueue.Refresh()
	var updates []capUpdate
	defer func() {
		pp.lock.Unlock()
		pp.updateFlags(updates)
	}()

	if c.connected {
		return
	}
	c.connected = true
	pp.inactiveQueue.Push(c, pp.inactivePriority(c))
	updates = pp.tryActivate()
}




func (pp *PriorityPool) disconnectedNode(c *ppNodeInfo) {
	pp.lock.Lock()
	pp.activeQueue.Refresh()
	var updates []capUpdate
	defer func() {
		pp.lock.Unlock()
		pp.updateFlags(updates)
	}()

	if !c.connected {
		return
	}
	c.connected = false
	pp.activeQueue.Remove(c.activeIndex)
	pp.inactiveQueue.Remove(c.inactiveIndex)
	if c.capacity != 0 {
		pp.setCapacity(c, 0)
		updates = pp.tryActivate()
	}
}





func (pp *PriorityPool) markForChange(c *ppNodeInfo) {
	if c.changed {
		return
	}
	c.changed = true
	c.origCap = c.capacity
	pp.changed = append(pp.changed, c)
}




func (pp *PriorityPool) setCapacity(n *ppNodeInfo, cap uint64) {
	pp.activeCap += cap - n.capacity
	if n.capacity == 0 {
		pp.activeCount++
	}
	if cap == 0 {
		pp.activeCount--
	}
	n.capacity = cap
}




func (pp *PriorityPool) enforceLimits() int64 {
	if pp.activeCap <= pp.maxCap && pp.activeCount <= pp.maxCount {
		return math.MinInt64
	}
	var maxActivePriority int64
	pp.activeQueue.MultiPop(func(data interface{}, priority int64) bool {
		c := data.(*ppNodeInfo)
		pp.markForChange(c)
		maxActivePriority = priority
		if c.capacity == pp.minCap {
			pp.setCapacity(c, 0)
		} else {
			sub := c.capacity / pp.capacityStepDiv
			if c.capacity-sub < pp.minCap {
				sub = c.capacity - pp.minCap
			}
			pp.setCapacity(c, c.capacity-sub)
			pp.activeQueue.Push(c)
		}
		return pp.activeCap > pp.maxCap || pp.activeCount > pp.maxCount
	})
	return invertPriority(maxActivePriority)
}




func (pp *PriorityPool) finalizeChanges(commit bool) (updates []capUpdate) {
	for _, c := range pp.changed {
		
		pp.activeQueue.Remove(c.activeIndex)
		pp.inactiveQueue.Remove(c.inactiveIndex)
		c.bias = 0
		c.forced = false
		c.changed = false
		if !commit {
			pp.setCapacity(c, c.origCap)
		}
		if c.connected {
			if c.capacity != 0 {
				pp.activeQueue.Push(c)
			} else {
				pp.inactiveQueue.Push(c, pp.inactivePriority(c))
			}
			if c.capacity != c.origCap && commit {
				updates = append(updates, capUpdate{c.node, c.origCap, c.capacity})
			}
		}
		c.origCap = 0
	}
	pp.changed = nil
	return
}


type capUpdate struct {
	node           *enode.Node
	oldCap, newCap uint64
}




func (pp *PriorityPool) updateFlags(updates []capUpdate) {
	for _, f := range updates {
		if f.oldCap == 0 {
			pp.ns.SetStateSub(f.node, pp.ActiveFlag, pp.InactiveFlag, 0)
		}
		if f.newCap == 0 {
			pp.ns.SetStateSub(f.node, pp.InactiveFlag, pp.ActiveFlag, 0)
			pp.ns.SetFieldSub(f.node, pp.CapacityField, nil)
		} else {
			pp.ns.SetFieldSub(f.node, pp.CapacityField, f.newCap)
		}
	}
}


func (pp *PriorityPool) tryActivate() []capUpdate {
	var commit bool
	for pp.inactiveQueue.Size() > 0 {
		c := pp.inactiveQueue.PopItem().(*ppNodeInfo)
		pp.markForChange(c)
		pp.setCapacity(c, pp.minCap)
		c.bias = pp.activeBias
		pp.activeQueue.Push(c)
		pp.enforceLimits()
		if c.capacity > 0 {
			commit = true
		} else {
			break
		}
	}
	return pp.finalizeChanges(commit)
}




func (pp *PriorityPool) updatePriority(node *enode.Node) {
	pp.lock.Lock()
	pp.activeQueue.Refresh()
	var updates []capUpdate
	defer func() {
		pp.lock.Unlock()
		pp.updateFlags(updates)
	}()

	c, _ := pp.ns.GetField(node, pp.ppNodeInfoField).(*ppNodeInfo)
	if c == nil || !c.connected {
		return
	}
	pp.activeQueue.Remove(c.activeIndex)
	pp.inactiveQueue.Remove(c.inactiveIndex)
	if c.capacity != 0 {
		pp.activeQueue.Push(c)
	} else {
		pp.inactiveQueue.Push(c, pp.inactivePriority(c))
	}
	updates = pp.tryActivate()
}
