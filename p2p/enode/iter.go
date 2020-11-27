















package enode

import (
	"sync"
	"time"
)




type Iterator interface {
	Next() bool  
	Node() *Node 
	Close()      
}




func ReadNodes(it Iterator, n int) []*Node {
	seen := make(map[ID]*Node, n)
	for i := 0; i < n && it.Next(); i++ {
		
		node := it.Node()
		prevNode, ok := seen[node.ID()]
		if ok && prevNode.Seq() > node.Seq() {
			continue
		}
		seen[node.ID()] = node
	}
	result := make([]*Node, 0, len(seen))
	for _, node := range seen {
		result = append(result, node)
	}
	return result
}


func IterNodes(nodes []*Node) Iterator {
	return &sliceIter{nodes: nodes, index: -1}
}


func CycleNodes(nodes []*Node) Iterator {
	return &sliceIter{nodes: nodes, index: -1, cycle: true}
}

type sliceIter struct {
	mu    sync.Mutex
	nodes []*Node
	index int
	cycle bool
}

func (it *sliceIter) Next() bool {
	it.mu.Lock()
	defer it.mu.Unlock()

	if len(it.nodes) == 0 {
		return false
	}
	it.index++
	if it.index == len(it.nodes) {
		if it.cycle {
			it.index = 0
		} else {
			it.nodes = nil
			return false
		}
	}
	return true
}

func (it *sliceIter) Node() *Node {
	it.mu.Lock()
	defer it.mu.Unlock()
	if len(it.nodes) == 0 {
		return nil
	}
	return it.nodes[it.index]
}

func (it *sliceIter) Close() {
	it.mu.Lock()
	defer it.mu.Unlock()

	it.nodes = nil
}



func Filter(it Iterator, check func(*Node) bool) Iterator {
	return &filterIter{it, check}
}

type filterIter struct {
	Iterator
	check func(*Node) bool
}

func (f *filterIter) Next() bool {
	for f.Iterator.Next() {
		if f.check(f.Node()) {
			return true
		}
	}
	return false
}











type FairMix struct {
	wg      sync.WaitGroup
	fromAny chan *Node
	timeout time.Duration
	cur     *Node

	mu      sync.Mutex
	closed  chan struct{}
	sources []*mixSource
	last    int
}

type mixSource struct {
	it      Iterator
	next    chan *Node
	timeout time.Duration
}







func NewFairMix(timeout time.Duration) *FairMix {
	m := &FairMix{
		fromAny: make(chan *Node),
		closed:  make(chan struct{}),
		timeout: timeout,
	}
	return m
}


func (m *FairMix) AddSource(it Iterator) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed == nil {
		return
	}
	m.wg.Add(1)
	source := &mixSource{it, make(chan *Node), m.timeout}
	m.sources = append(m.sources, source)
	go m.runSource(m.closed, source)
}



func (m *FairMix) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed == nil {
		return
	}
	for _, s := range m.sources {
		s.it.Close()
	}
	close(m.closed)
	m.wg.Wait()
	close(m.fromAny)
	m.sources = nil
	m.closed = nil
}


func (m *FairMix) Next() bool {
	m.cur = nil

	var timeout <-chan time.Time
	if m.timeout >= 0 {
		timer := time.NewTimer(m.timeout)
		timeout = timer.C
		defer timer.Stop()
	}
	for {
		source := m.pickSource()
		if source == nil {
			return m.nextFromAny()
		}
		select {
		case n, ok := <-source.next:
			if ok {
				m.cur = n
				source.timeout = m.timeout
				return true
			}
			
			m.deleteSource(source)
		case <-timeout:
			source.timeout /= 2
			return m.nextFromAny()
		}
	}
}


func (m *FairMix) Node() *Node {
	return m.cur
}



func (m *FairMix) nextFromAny() bool {
	n, ok := <-m.fromAny
	if ok {
		m.cur = n
	}
	return ok
}


func (m *FairMix) pickSource() *mixSource {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sources) == 0 {
		return nil
	}
	m.last = (m.last + 1) % len(m.sources)
	return m.sources[m.last]
}


func (m *FairMix) deleteSource(s *mixSource) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.sources {
		if m.sources[i] == s {
			copy(m.sources[i:], m.sources[i+1:])
			m.sources[len(m.sources)-1] = nil
			m.sources = m.sources[:len(m.sources)-1]
			break
		}
	}
}


func (m *FairMix) runSource(closed chan struct{}, s *mixSource) {
	defer m.wg.Done()
	defer close(s.next)
	for s.it.Next() {
		n := s.it.Node()
		select {
		case s.next <- n:
		case m.fromAny <- n:
		case <-closed:
			return
		}
	}
}
