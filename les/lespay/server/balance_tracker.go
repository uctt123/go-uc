















package server

import (
	"reflect"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nodestate"
)

const (
	posThreshold             = 1000000         
	negThreshold             = 1000000         
	persistExpirationRefresh = time.Minute * 5 
)


type BalanceTrackerSetup struct {
	
	PriorityFlag, UpdateFlag nodestate.Flags
	BalanceField             nodestate.Field
	
	connAddressField, capacityField nodestate.Field
}



func NewBalanceTrackerSetup(setup *nodestate.Setup) BalanceTrackerSetup {
	return BalanceTrackerSetup{
		
		PriorityFlag: setup.NewFlag("priorityNode"),
		
		
		UpdateFlag: setup.NewFlag("balanceUpdate"),
		
		
		BalanceField: setup.NewField("balance", reflect.TypeOf(&NodeBalance{})),
	}
}


func (bts *BalanceTrackerSetup) Connect(connAddressField, capacityField nodestate.Field) {
	bts.connAddressField = connAddressField
	bts.capacityField = capacityField
}









type BalanceTracker struct {
	BalanceTrackerSetup
	clock              mclock.Clock
	lock               sync.Mutex
	ns                 *nodestate.NodeStateMachine
	ndb                *nodeDB
	posExp, negExp     utils.ValueExpirer
	posExpTC, negExpTC uint64

	active, inactive utils.ExpiredValue
	balanceTimer     *utils.UpdateTimer
	quit             chan struct{}
}


func NewBalanceTracker(ns *nodestate.NodeStateMachine, setup BalanceTrackerSetup, db ethdb.KeyValueStore, clock mclock.Clock, posExp, negExp utils.ValueExpirer) *BalanceTracker {
	ndb := newNodeDB(db, clock)
	bt := &BalanceTracker{
		ns:                  ns,
		BalanceTrackerSetup: setup,
		ndb:                 ndb,
		clock:               clock,
		posExp:              posExp,
		negExp:              negExp,
		balanceTimer:        utils.NewUpdateTimer(clock, time.Second*10),
		quit:                make(chan struct{}),
	}
	bt.ndb.forEachBalance(false, func(id enode.ID, balance utils.ExpiredValue) bool {
		bt.inactive.AddExp(balance)
		return true
	})

	ns.SubscribeField(bt.capacityField, func(node *enode.Node, state nodestate.Flags, oldValue, newValue interface{}) {
		n, _ := ns.GetField(node, bt.BalanceField).(*NodeBalance)
		if n == nil {
			return
		}

		ov, _ := oldValue.(uint64)
		nv, _ := newValue.(uint64)
		if ov == 0 && nv != 0 {
			n.activate()
		}
		if nv != 0 {
			n.setCapacity(nv)
		}
		if ov != 0 && nv == 0 {
			n.deactivate()
		}
	})
	ns.SubscribeField(bt.connAddressField, func(node *enode.Node, state nodestate.Flags, oldValue, newValue interface{}) {
		if newValue != nil {
			ns.SetFieldSub(node, bt.BalanceField, bt.newNodeBalance(node, newValue.(string)))
		} else {
			ns.SetStateSub(node, nodestate.Flags{}, bt.PriorityFlag, 0)
			if b, _ := ns.GetField(node, bt.BalanceField).(*NodeBalance); b != nil {
				b.deactivate()
			}
			ns.SetFieldSub(node, bt.BalanceField, nil)
		}
	})

	
	
	
	bt.ndb.evictCallBack = bt.canDropBalance

	go func() {
		for {
			select {
			case <-clock.After(persistExpirationRefresh):
				now := clock.Now()
				bt.ndb.setExpiration(posExp.LogOffset(now), negExp.LogOffset(now))
			case <-bt.quit:
				return
			}
		}
	}()
	return bt
}


func (bt *BalanceTracker) Stop() {
	now := bt.clock.Now()
	bt.ndb.setExpiration(bt.posExp.LogOffset(now), bt.negExp.LogOffset(now))
	close(bt.quit)
	bt.ns.ForEach(nodestate.Flags{}, nodestate.Flags{}, func(node *enode.Node, state nodestate.Flags) {
		if n, ok := bt.ns.GetField(node, bt.BalanceField).(*NodeBalance); ok {
			n.lock.Lock()
			n.storeBalance(true, true)
			n.lock.Unlock()
			bt.ns.SetField(node, bt.BalanceField, nil)
		}
	})
	bt.ndb.close()
}


func (bt *BalanceTracker) TotalTokenAmount() uint64 {
	bt.lock.Lock()
	defer bt.lock.Unlock()

	bt.balanceTimer.Update(func(_ time.Duration) bool {
		bt.active = utils.ExpiredValue{}
		bt.ns.ForEach(nodestate.Flags{}, nodestate.Flags{}, func(node *enode.Node, state nodestate.Flags) {
			if n, ok := bt.ns.GetField(node, bt.BalanceField).(*NodeBalance); ok {
				pos, _ := n.GetRawBalance()
				bt.active.AddExp(pos)
			}
		})
		return true
	})
	total := bt.active
	total.AddExp(bt.inactive)
	return total.Value(bt.posExp.LogOffset(bt.clock.Now()))
}


func (bt *BalanceTracker) GetPosBalanceIDs(start, stop enode.ID, maxCount int) (result []enode.ID) {
	return bt.ndb.getPosBalanceIDs(start, stop, maxCount)
}



func (bt *BalanceTracker) SetExpirationTCs(pos, neg uint64) {
	bt.lock.Lock()
	defer bt.lock.Unlock()

	bt.posExpTC, bt.negExpTC = pos, neg
	now := bt.clock.Now()
	if pos > 0 {
		bt.posExp.SetRate(now, 1/float64(pos*uint64(time.Second)))
	} else {
		bt.posExp.SetRate(now, 0)
	}
	if neg > 0 {
		bt.negExp.SetRate(now, 1/float64(neg*uint64(time.Second)))
	} else {
		bt.negExp.SetRate(now, 0)
	}
}



func (bt *BalanceTracker) GetExpirationTCs() (pos, neg uint64) {
	bt.lock.Lock()
	defer bt.lock.Unlock()

	return bt.posExpTC, bt.negExpTC
}





func (bt *BalanceTracker) newNodeBalance(node *enode.Node, negBalanceKey string) *NodeBalance {
	pb := bt.ndb.getOrNewBalance(node.ID().Bytes(), false)
	nb := bt.ndb.getOrNewBalance([]byte(negBalanceKey), true)
	n := &NodeBalance{
		bt:          bt,
		node:        node,
		connAddress: negBalanceKey,
		balance:     balance{pos: pb, neg: nb},
		initTime:    bt.clock.Now(),
		lastUpdate:  bt.clock.Now(),
	}
	for i := range n.callbackIndex {
		n.callbackIndex[i] = -1
	}
	if n.checkPriorityStatus() {
		n.bt.ns.SetStateSub(n.node, n.bt.PriorityFlag, nodestate.Flags{}, 0)
	}
	return n
}


func (bt *BalanceTracker) storeBalance(id []byte, neg bool, value utils.ExpiredValue) {
	if bt.canDropBalance(bt.clock.Now(), neg, value) {
		bt.ndb.delBalance(id, neg) 
	} else {
		bt.ndb.setBalance(id, neg, value)
	}
}



func (bt *BalanceTracker) canDropBalance(now mclock.AbsTime, neg bool, b utils.ExpiredValue) bool {
	if neg {
		return b.Value(bt.negExp.LogOffset(now)) <= negThreshold
	} else {
		return b.Value(bt.posExp.LogOffset(now)) <= posThreshold
	}
}


func (bt *BalanceTracker) updateTotalBalance(n *NodeBalance, callback func() bool) {
	bt.lock.Lock()
	defer bt.lock.Unlock()

	n.lock.Lock()
	defer n.lock.Unlock()

	original, active := n.balance.pos, n.active
	if !callback() {
		return
	}
	if active {
		bt.active.SubExp(original)
	} else {
		bt.inactive.SubExp(original)
	}
	if n.active {
		bt.active.AddExp(n.balance.pos)
	} else {
		bt.inactive.AddExp(n.balance.pos)
	}
}
