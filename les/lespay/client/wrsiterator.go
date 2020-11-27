















package client

import (
	"sync"

	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nodestate"
)



type WrsIterator struct {
	lock sync.Mutex
	cond *sync.Cond

	ns       *nodestate.NodeStateMachine
	wrs      *utils.WeightedRandomSelect
	nextNode *enode.Node
	closed   bool
}




func NewWrsIterator(ns *nodestate.NodeStateMachine, requireFlags, disableFlags nodestate.Flags, weightField nodestate.Field) *WrsIterator {
	wfn := func(i interface{}) uint64 {
		n := ns.GetNode(i.(enode.ID))
		if n == nil {
			return 0
		}
		wt, _ := ns.GetField(n, weightField).(uint64)
		return wt
	}

	w := &WrsIterator{
		ns:  ns,
		wrs: utils.NewWeightedRandomSelect(wfn),
	}
	w.cond = sync.NewCond(&w.lock)

	ns.SubscribeField(weightField, func(n *enode.Node, state nodestate.Flags, oldValue, newValue interface{}) {
		if state.HasAll(requireFlags) && state.HasNone(disableFlags) {
			w.lock.Lock()
			w.wrs.Update(n.ID())
			w.lock.Unlock()
			w.cond.Signal()
		}
	})

	ns.SubscribeState(requireFlags.Or(disableFlags), func(n *enode.Node, oldState, newState nodestate.Flags) {
		oldMatch := oldState.HasAll(requireFlags) && oldState.HasNone(disableFlags)
		newMatch := newState.HasAll(requireFlags) && newState.HasNone(disableFlags)
		if newMatch == oldMatch {
			return
		}

		w.lock.Lock()
		if newMatch {
			w.wrs.Update(n.ID())
		} else {
			w.wrs.Remove(n.ID())
		}
		w.lock.Unlock()
		w.cond.Signal()
	})
	return w
}


func (w *WrsIterator) Next() bool {
	w.nextNode = w.chooseNode()
	return w.nextNode != nil
}

func (w *WrsIterator) chooseNode() *enode.Node {
	w.lock.Lock()
	defer w.lock.Unlock()

	for {
		for !w.closed && w.wrs.IsEmpty() {
			w.cond.Wait()
		}
		if w.closed {
			return nil
		}
		
		
		
		if c := w.wrs.Choose(); c != nil {
			id := c.(enode.ID)
			w.wrs.Remove(id)
			return w.ns.GetNode(id)
		}
	}

}


func (w *WrsIterator) Close() {
	w.lock.Lock()
	w.closed = true
	w.lock.Unlock()
	w.cond.Signal()
}


func (w *WrsIterator) Node() *enode.Node {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.nextNode
}
