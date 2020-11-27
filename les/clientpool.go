















package les

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/ethdb"
	lps "github.com/ethereum/go-ethereum/les/lespay/server"
	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/p2p/nodestate"
)

const (
	defaultNegExpTC = 3600 

	
	
	
	
	
	
	
	defaultConnectedBias = time.Minute * 3
	inactiveTimeout      = time.Second * 10
)

var (
	clientPoolSetup     = &nodestate.Setup{}
	clientField         = clientPoolSetup.NewField("clientInfo", reflect.TypeOf(&clientInfo{}))
	connAddressField    = clientPoolSetup.NewField("connAddr", reflect.TypeOf(""))
	balanceTrackerSetup = lps.NewBalanceTrackerSetup(clientPoolSetup)
	priorityPoolSetup   = lps.NewPriorityPoolSetup(clientPoolSetup)
)

func init() {
	balanceTrackerSetup.Connect(connAddressField, priorityPoolSetup.CapacityField)
	priorityPoolSetup.Connect(balanceTrackerSetup.BalanceField, balanceTrackerSetup.UpdateFlag) 
}



















type clientPool struct {
	lps.BalanceTrackerSetup
	lps.PriorityPoolSetup
	lock       sync.Mutex
	clock      mclock.Clock
	closed     bool
	removePeer func(enode.ID)
	ns         *nodestate.NodeStateMachine
	pp         *lps.PriorityPool
	bt         *lps.BalanceTracker

	defaultPosFactors, defaultNegFactors lps.PriceFactors
	posExpTC, negExpTC                   uint64
	minCap                               uint64 
	connectedBias                        time.Duration
	capLimit                             uint64
}






type clientPoolPeer interface {
	Node() *enode.Node
	freeClientId() string
	updateCapacity(uint64)
	freeze()
	allowInactive() bool
}


type clientInfo struct {
	node                *enode.Node
	address             string
	peer                clientPoolPeer
	connected, priority bool
	connectedAt         mclock.AbsTime
	balance             *lps.NodeBalance
}


func newClientPool(lespayDb ethdb.Database, minCap uint64, connectedBias time.Duration, clock mclock.Clock, removePeer func(enode.ID)) *clientPool {
	ns := nodestate.NewNodeStateMachine(nil, nil, clock, clientPoolSetup)
	pool := &clientPool{
		ns:                  ns,
		BalanceTrackerSetup: balanceTrackerSetup,
		PriorityPoolSetup:   priorityPoolSetup,
		clock:               clock,
		minCap:              minCap,
		connectedBias:       connectedBias,
		removePeer:          removePeer,
	}
	pool.bt = lps.NewBalanceTracker(ns, balanceTrackerSetup, lespayDb, clock, &utils.Expirer{}, &utils.Expirer{})
	pool.pp = lps.NewPriorityPool(ns, priorityPoolSetup, clock, minCap, connectedBias, 4)

	
	
	pool.bt.SetExpirationTCs(0, defaultNegExpTC)

	ns.SubscribeState(pool.InactiveFlag.Or(pool.PriorityFlag), func(node *enode.Node, oldState, newState nodestate.Flags) {
		if newState.Equals(pool.InactiveFlag) {
			ns.AddTimeout(node, pool.InactiveFlag, inactiveTimeout)
		}
		if oldState.Equals(pool.InactiveFlag) && newState.Equals(pool.InactiveFlag.Or(pool.PriorityFlag)) {
			ns.SetStateSub(node, pool.InactiveFlag, nodestate.Flags{}, 0) 
		}
	})

	ns.SubscribeState(pool.ActiveFlag.Or(pool.PriorityFlag), func(node *enode.Node, oldState, newState nodestate.Flags) {
		c, _ := ns.GetField(node, clientField).(*clientInfo)
		if c == nil {
			return
		}
		c.priority = newState.HasAll(pool.PriorityFlag)
		if newState.Equals(pool.ActiveFlag) {
			cap, _ := ns.GetField(node, pool.CapacityField).(uint64)
			if cap > minCap {
				pool.pp.RequestCapacity(node, minCap, 0, true)
			}
		}
	})

	ns.SubscribeState(pool.InactiveFlag.Or(pool.ActiveFlag), func(node *enode.Node, oldState, newState nodestate.Flags) {
		if oldState.IsEmpty() {
			clientConnectedMeter.Mark(1)
			log.Debug("Client connected", "id", node.ID())
		}
		if oldState.Equals(pool.InactiveFlag) && newState.Equals(pool.ActiveFlag) {
			clientActivatedMeter.Mark(1)
			log.Debug("Client activated", "id", node.ID())
		}
		if oldState.Equals(pool.ActiveFlag) && newState.Equals(pool.InactiveFlag) {
			clientDeactivatedMeter.Mark(1)
			log.Debug("Client deactivated", "id", node.ID())
			c, _ := ns.GetField(node, clientField).(*clientInfo)
			if c == nil || !c.peer.allowInactive() {
				pool.removePeer(node.ID())
			}
		}
		if newState.IsEmpty() {
			clientDisconnectedMeter.Mark(1)
			log.Debug("Client disconnected", "id", node.ID())
			pool.removePeer(node.ID())
		}
	})

	var totalConnected uint64
	ns.SubscribeField(pool.CapacityField, func(node *enode.Node, state nodestate.Flags, oldValue, newValue interface{}) {
		oldCap, _ := oldValue.(uint64)
		newCap, _ := newValue.(uint64)
		totalConnected += newCap - oldCap
		totalConnectedGauge.Update(int64(totalConnected))
		c, _ := ns.GetField(node, clientField).(*clientInfo)
		if c != nil {
			c.peer.updateCapacity(newCap)
		}
	})

	ns.Start()
	return pool
}


func (f *clientPool) stop() {
	f.lock.Lock()
	f.closed = true
	f.lock.Unlock()
	f.ns.ForEach(nodestate.Flags{}, nodestate.Flags{}, func(node *enode.Node, state nodestate.Flags) {
		
		f.disconnectNode(node)
	})
	f.bt.Stop()
	f.ns.Stop()
}



func (f *clientPool) connect(peer clientPoolPeer) (uint64, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	
	if f.closed {
		return 0, fmt.Errorf("Client pool is already closed")
	}
	
	node, freeID := peer.Node(), peer.freeClientId()
	if f.ns.GetField(node, clientField) != nil {
		log.Debug("Client already connected", "address", freeID, "id", node.ID().String())
		return 0, fmt.Errorf("Client already connected address=%s id=%s", freeID, node.ID().String())
	}
	now := f.clock.Now()
	c := &clientInfo{
		node:        node,
		address:     freeID,
		peer:        peer,
		connected:   true,
		connectedAt: now,
	}
	f.ns.SetField(node, clientField, c)
	f.ns.SetField(node, connAddressField, freeID)
	if c.balance, _ = f.ns.GetField(node, f.BalanceField).(*lps.NodeBalance); c.balance == nil {
		f.disconnect(peer)
		return 0, nil
	}
	c.balance.SetPriceFactors(f.defaultPosFactors, f.defaultNegFactors)

	f.ns.SetState(node, f.InactiveFlag, nodestate.Flags{}, 0)
	var allowed bool
	f.ns.Operation(func() {
		_, allowed = f.pp.RequestCapacity(node, f.minCap, f.connectedBias, true)
	})
	if allowed {
		return f.minCap, nil
	}
	if !peer.allowInactive() {
		f.disconnect(peer)
	}
	return 0, nil
}




func (f *clientPool) setConnectedBias(bias time.Duration) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.connectedBias = bias
	f.pp.SetActiveBias(bias)
}




func (f *clientPool) disconnect(p clientPoolPeer) {
	f.disconnectNode(p.Node())
}


func (f *clientPool) disconnectNode(node *enode.Node) {
	f.ns.SetField(node, connAddressField, nil)
	f.ns.SetField(node, clientField, nil)
}


func (f *clientPool) setDefaultFactors(posFactors, negFactors lps.PriceFactors) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.defaultPosFactors = posFactors
	f.defaultNegFactors = negFactors
}



func (f *clientPool) capacityInfo() (uint64, uint64, uint64) {
	f.lock.Lock()
	defer f.lock.Unlock()

	
	return f.capLimit, f.pp.ActiveCapacity(), 0
}



func (f *clientPool) setLimits(totalConn int, totalCap uint64) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.capLimit = totalCap
	f.pp.SetLimits(uint64(totalConn), totalCap)
}


func (f *clientPool) setCapacity(node *enode.Node, freeID string, capacity uint64, bias time.Duration, setCap bool) (uint64, error) {
	c, _ := f.ns.GetField(node, clientField).(*clientInfo)
	if c == nil {
		if setCap {
			return 0, fmt.Errorf("client %064x is not connected", node.ID())
		}
		c = &clientInfo{node: node}
		f.ns.SetField(node, clientField, c)
		f.ns.SetField(node, connAddressField, freeID)
		if c.balance, _ = f.ns.GetField(node, f.BalanceField).(*lps.NodeBalance); c.balance == nil {
			log.Error("BalanceField is missing", "node", node.ID())
			return 0, fmt.Errorf("BalanceField of %064x is missing", node.ID())
		}
		defer func() {
			f.ns.SetField(node, connAddressField, nil)
			f.ns.SetField(node, clientField, nil)
		}()
	}
	var (
		minPriority int64
		allowed     bool
	)
	f.ns.Operation(func() {
		if !setCap || c.priority {
			
			minPriority, allowed = f.pp.RequestCapacity(node, capacity, bias, setCap)
		}
	})
	if allowed {
		return 0, nil
	}
	missing := c.balance.PosBalanceMissing(minPriority, capacity, bias)
	if missing < 1 {
		
		missing = 1
	}
	return missing, errNoPriority
}


func (f *clientPool) setCapacityLocked(node *enode.Node, freeID string, capacity uint64, minConnTime time.Duration, setCap bool) (uint64, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.setCapacity(node, freeID, capacity, minConnTime, setCap)
}





func (f *clientPool) forClients(ids []enode.ID, cb func(client *clientInfo)) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if len(ids) == 0 {
		f.ns.ForEach(nodestate.Flags{}, nodestate.Flags{}, func(node *enode.Node, state nodestate.Flags) {
			c, _ := f.ns.GetField(node, clientField).(*clientInfo)
			if c != nil {
				cb(c)
			}
		})
	} else {
		for _, id := range ids {
			node := f.ns.GetNode(id)
			if node == nil {
				node = enode.SignNull(&enr.Record{}, id)
			}
			c, _ := f.ns.GetField(node, clientField).(*clientInfo)
			if c != nil {
				cb(c)
			} else {
				c = &clientInfo{node: node}
				f.ns.SetField(node, clientField, c)
				f.ns.SetField(node, connAddressField, "")
				if c.balance, _ = f.ns.GetField(node, f.BalanceField).(*lps.NodeBalance); c.balance != nil {
					cb(c)
				} else {
					log.Error("BalanceField is missing")
				}
				f.ns.SetField(node, connAddressField, nil)
				f.ns.SetField(node, clientField, nil)
			}
		}
	}
}
