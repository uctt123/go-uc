















package client

import (
	"sync"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nodestate"
)






type FillSet struct {
	lock          sync.Mutex
	cond          *sync.Cond
	ns            *nodestate.NodeStateMachine
	input         enode.Iterator
	closed        bool
	flags         nodestate.Flags
	count, target int
}


func NewFillSet(ns *nodestate.NodeStateMachine, input enode.Iterator, flags nodestate.Flags) *FillSet {
	fs := &FillSet{
		ns:    ns,
		input: input,
		flags: flags,
	}
	fs.cond = sync.NewCond(&fs.lock)

	ns.SubscribeState(flags, func(n *enode.Node, oldState, newState nodestate.Flags) {
		fs.lock.Lock()
		if oldState.Equals(flags) {
			fs.count--
		}
		if newState.Equals(flags) {
			fs.count++
		}
		if fs.target > fs.count {
			fs.cond.Signal()
		}
		fs.lock.Unlock()
	})

	go fs.readLoop()
	return fs
}



func (fs *FillSet) readLoop() {
	for {
		fs.lock.Lock()
		for fs.target <= fs.count && !fs.closed {
			fs.cond.Wait()
		}

		fs.lock.Unlock()
		if !fs.input.Next() {
			return
		}
		fs.ns.SetState(fs.input.Node(), fs.flags, nodestate.Flags{}, 0)
	}
}





func (fs *FillSet) SetTarget(target int) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	fs.target = target
	if fs.target > fs.count {
		fs.cond.Signal()
	}
}


func (fs *FillSet) Close() {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	fs.closed = true
	fs.input.Close()
	fs.cond.Signal()
}
