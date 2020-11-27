















package discover

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/p2p/enode"
)




type lookup struct {
	tab         *Table
	queryfunc   func(*node) ([]*node, error)
	replyCh     chan []*node
	cancelCh    <-chan struct{}
	asked, seen map[enode.ID]bool
	result      nodesByDistance
	replyBuffer []*node
	queries     int
}

type queryFunc func(*node) ([]*node, error)

func newLookup(ctx context.Context, tab *Table, target enode.ID, q queryFunc) *lookup {
	it := &lookup{
		tab:       tab,
		queryfunc: q,
		asked:     make(map[enode.ID]bool),
		seen:      make(map[enode.ID]bool),
		result:    nodesByDistance{target: target},
		replyCh:   make(chan []*node, alpha),
		cancelCh:  ctx.Done(),
		queries:   -1,
	}
	
	
	it.asked[tab.self().ID()] = true
	return it
}


func (it *lookup) run() []*enode.Node {
	for it.advance() {
	}
	return unwrapNodes(it.result.entries)
}



func (it *lookup) advance() bool {
	for it.startQueries() {
		select {
		case nodes := <-it.replyCh:
			it.replyBuffer = it.replyBuffer[:0]
			for _, n := range nodes {
				if n != nil && !it.seen[n.ID()] {
					it.seen[n.ID()] = true
					it.result.push(n, bucketSize)
					it.replyBuffer = append(it.replyBuffer, n)
				}
			}
			it.queries--
			if len(it.replyBuffer) > 0 {
				return true
			}
		case <-it.cancelCh:
			it.shutdown()
		}
	}
	return false
}

func (it *lookup) shutdown() {
	for it.queries > 0 {
		<-it.replyCh
		it.queries--
	}
	it.queryfunc = nil
	it.replyBuffer = nil
}

func (it *lookup) startQueries() bool {
	if it.queryfunc == nil {
		return false
	}

	
	if it.queries == -1 {
		closest := it.tab.findnodeByID(it.result.target, bucketSize, false)
		
		
		
		if len(closest.entries) == 0 {
			it.slowdown()
		}
		it.queries = 1
		it.replyCh <- closest.entries
		return true
	}

	
	for i := 0; i < len(it.result.entries) && it.queries < alpha; i++ {
		n := it.result.entries[i]
		if !it.asked[n.ID()] {
			it.asked[n.ID()] = true
			it.queries++
			go it.query(n, it.replyCh)
		}
	}
	
	return it.queries > 0
}

func (it *lookup) slowdown() {
	sleep := time.NewTimer(1 * time.Second)
	defer sleep.Stop()
	select {
	case <-sleep.C:
	case <-it.tab.closeReq:
	}
}

func (it *lookup) query(n *node, reply chan<- []*node) {
	fails := it.tab.db.FindFails(n.ID(), n.IP())
	r, err := it.queryfunc(n)
	if err == errClosed {
		
		reply <- nil
		return
	} else if len(r) == 0 {
		fails++
		it.tab.db.UpdateFindFails(n.ID(), n.IP(), fails)
		
		
		dropped := false
		if fails >= maxFindnodeFailures && it.tab.bucketLen(n.ID()) >= bucketSize/2 {
			dropped = true
			it.tab.delete(n)
		}
		it.tab.log.Trace("FINDNODE failed", "id", n.ID(), "failcount", fails, "dropped", dropped, "err", err)
	} else if fails > 0 {
		
		it.tab.db.UpdateFindFails(n.ID(), n.IP(), 0)
	}

	
	
	for _, n := range r {
		it.tab.addSeenNode(n)
	}
	reply <- r
}



type lookupIterator struct {
	buffer     []*node
	nextLookup lookupFunc
	ctx        context.Context
	cancel     func()
	lookup     *lookup
}

type lookupFunc func(ctx context.Context) *lookup

func newLookupIterator(ctx context.Context, next lookupFunc) *lookupIterator {
	ctx, cancel := context.WithCancel(ctx)
	return &lookupIterator{ctx: ctx, cancel: cancel, nextLookup: next}
}


func (it *lookupIterator) Node() *enode.Node {
	if len(it.buffer) == 0 {
		return nil
	}
	return unwrapNode(it.buffer[0])
}


func (it *lookupIterator) Next() bool {
	
	if len(it.buffer) > 0 {
		it.buffer = it.buffer[1:]
	}
	
	for len(it.buffer) == 0 {
		if it.ctx.Err() != nil {
			it.lookup = nil
			it.buffer = nil
			return false
		}
		if it.lookup == nil {
			it.lookup = it.nextLookup(it.ctx)
			continue
		}
		if !it.lookup.advance() {
			it.lookup = nil
			continue
		}
		it.buffer = it.lookup.replyBuffer
	}
	return true
}


func (it *lookupIterator) Close() {
	it.cancel()
}
