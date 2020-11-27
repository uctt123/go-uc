















package client

import (
	"sync"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nodestate"
)



type QueueIterator struct {
	lock sync.Mutex
	cond *sync.Cond

	ns           *nodestate.NodeStateMachine
	queue        []*enode.Node
	nextNode     *enode.Node
	waitCallback func(bool)
	fifo, closed bool
}




func NewQueueIterator(ns *nodestate.NodeStateMachine, requireFlags, disableFlags nodestate.Flags, fifo bool, waitCallback func(bool)) *QueueIterator {
	qi := &QueueIterator{
		ns:           ns,
		fifo:         fifo,
		waitCallback: waitCallback,
	}
	qi.cond = sync.NewCond(&qi.lock)

	ns.SubscribeState(requireFlags.Or(disableFlags), func(n *enode.Node, oldState, newState nodestate.Flags) {
		oldMatch := oldState.HasAll(requireFlags) && oldState.HasNone(disableFlags)
		newMatch := newState.HasAll(requireFlags) && newState.HasNone(disableFlags)
		if newMatch == oldMatch {
			return
		}

		qi.lock.Lock()
		defer qi.lock.Unlock()

		if newMatch {
			qi.queue = append(qi.queue, n)
		} else {
			id := n.ID()
			for i, qn := range qi.queue {
				if qn.ID() == id {
					copy(qi.queue[i:len(qi.queue)-1], qi.queue[i+1:])
					qi.queue = qi.queue[:len(qi.queue)-1]
					break
				}
			}
		}
		qi.cond.Signal()
	})
	return qi
}


func (qi *QueueIterator) Next() bool {
	qi.lock.Lock()
	if !qi.closed && len(qi.queue) == 0 {
		if qi.waitCallback != nil {
			qi.waitCallback(true)
		}
		for !qi.closed && len(qi.queue) == 0 {
			qi.cond.Wait()
		}
		if qi.waitCallback != nil {
			qi.waitCallback(false)
		}
	}
	if qi.closed {
		qi.nextNode = nil
		qi.lock.Unlock()
		return false
	}
	
	if qi.fifo {
		qi.nextNode = qi.queue[0]
		copy(qi.queue[:len(qi.queue)-1], qi.queue[1:])
		qi.queue = qi.queue[:len(qi.queue)-1]
	} else {
		qi.nextNode = qi.queue[len(qi.queue)-1]
		qi.queue = qi.queue[:len(qi.queue)-1]
	}
	qi.lock.Unlock()
	return true
}


func (qi *QueueIterator) Close() {
	qi.lock.Lock()
	qi.closed = true
	qi.lock.Unlock()
	qi.cond.Signal()
}


func (qi *QueueIterator) Node() *enode.Node {
	qi.lock.Lock()
	defer qi.lock.Unlock()

	return qi.nextNode
}
