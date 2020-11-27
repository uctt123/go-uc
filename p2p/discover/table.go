





















package discover

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	mrand "math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)

const (
	alpha           = 3  
	bucketSize      = 16 
	maxReplacements = 10 

	
	
	hashBits          = len(common.Hash{}) * 8
	nBuckets          = hashBits / 15       
	bucketMinDistance = hashBits - nBuckets 

	
	bucketIPLimit, bucketSubnet = 2, 24 
	tableIPLimit, tableSubnet   = 10, 24

	refreshInterval    = 30 * time.Minute
	revalidateInterval = 10 * time.Second
	copyNodesInterval  = 30 * time.Second
	seedMinTableTime   = 5 * time.Minute
	seedCount          = 30
	seedMaxAge         = 5 * 24 * time.Hour
)




type Table struct {
	mutex   sync.Mutex        
	buckets [nBuckets]*bucket 
	nursery []*node           
	rand    *mrand.Rand       
	ips     netutil.DistinctNetSet

	log        log.Logger
	db         *enode.DB 
	net        transport
	refreshReq chan chan struct{}
	initDone   chan struct{}
	closeReq   chan struct{}
	closed     chan struct{}

	nodeAddedHook func(*node) 
}


type transport interface {
	Self() *enode.Node
	RequestENR(*enode.Node) (*enode.Node, error)
	lookupRandom() []*enode.Node
	lookupSelf() []*enode.Node
	ping(*enode.Node) (seq uint64, err error)
}



type bucket struct {
	entries      []*node 
	replacements []*node 
	ips          netutil.DistinctNetSet
}

func newTable(t transport, db *enode.DB, bootnodes []*enode.Node, log log.Logger) (*Table, error) {
	tab := &Table{
		net:        t,
		db:         db,
		refreshReq: make(chan chan struct{}),
		initDone:   make(chan struct{}),
		closeReq:   make(chan struct{}),
		closed:     make(chan struct{}),
		rand:       mrand.New(mrand.NewSource(0)),
		ips:        netutil.DistinctNetSet{Subnet: tableSubnet, Limit: tableIPLimit},
		log:        log,
	}
	if err := tab.setFallbackNodes(bootnodes); err != nil {
		return nil, err
	}
	for i := range tab.buckets {
		tab.buckets[i] = &bucket{
			ips: netutil.DistinctNetSet{Subnet: bucketSubnet, Limit: bucketIPLimit},
		}
	}
	tab.seedRand()
	tab.loadSeedNodes()

	return tab, nil
}

func (tab *Table) self() *enode.Node {
	return tab.net.Self()
}

func (tab *Table) seedRand() {
	var b [8]byte
	crand.Read(b[:])

	tab.mutex.Lock()
	tab.rand.Seed(int64(binary.BigEndian.Uint64(b[:])))
	tab.mutex.Unlock()
}



func (tab *Table) ReadRandomNodes(buf []*enode.Node) (n int) {
	if !tab.isInitDone() {
		return 0
	}
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	var nodes []*enode.Node
	for _, b := range &tab.buckets {
		for _, n := range b.entries {
			nodes = append(nodes, unwrapNode(n))
		}
	}
	
	for i := 0; i < len(nodes); i++ {
		j := tab.rand.Intn(len(nodes))
		nodes[i], nodes[j] = nodes[j], nodes[i]
	}
	return copy(buf, nodes)
}


func (tab *Table) getNode(id enode.ID) *enode.Node {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	b := tab.bucket(id)
	for _, e := range b.entries {
		if e.ID() == id {
			return unwrapNode(e)
		}
	}
	return nil
}


func (tab *Table) close() {
	close(tab.closeReq)
	<-tab.closed
}




func (tab *Table) setFallbackNodes(nodes []*enode.Node) error {
	for _, n := range nodes {
		if err := n.ValidateComplete(); err != nil {
			return fmt.Errorf("bad bootstrap node %q: %v", n, err)
		}
	}
	tab.nursery = wrapNodes(nodes)
	return nil
}


func (tab *Table) isInitDone() bool {
	select {
	case <-tab.initDone:
		return true
	default:
		return false
	}
}

func (tab *Table) refresh() <-chan struct{} {
	done := make(chan struct{})
	select {
	case tab.refreshReq <- done:
	case <-tab.closeReq:
		close(done)
	}
	return done
}


func (tab *Table) loop() {
	var (
		revalidate     = time.NewTimer(tab.nextRevalidateTime())
		refresh        = time.NewTicker(refreshInterval)
		copyNodes      = time.NewTicker(copyNodesInterval)
		refreshDone    = make(chan struct{})           
		revalidateDone chan struct{}                   
		waiting        = []chan struct{}{tab.initDone} 
	)
	defer refresh.Stop()
	defer revalidate.Stop()
	defer copyNodes.Stop()

	
	go tab.doRefresh(refreshDone)

loop:
	for {
		select {
		case <-refresh.C:
			tab.seedRand()
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case req := <-tab.refreshReq:
			waiting = append(waiting, req)
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case <-refreshDone:
			for _, ch := range waiting {
				close(ch)
			}
			waiting, refreshDone = nil, nil
		case <-revalidate.C:
			revalidateDone = make(chan struct{})
			go tab.doRevalidate(revalidateDone)
		case <-revalidateDone:
			revalidate.Reset(tab.nextRevalidateTime())
			revalidateDone = nil
		case <-copyNodes.C:
			go tab.copyLiveNodes()
		case <-tab.closeReq:
			break loop
		}
	}

	if refreshDone != nil {
		<-refreshDone
	}
	for _, ch := range waiting {
		close(ch)
	}
	if revalidateDone != nil {
		<-revalidateDone
	}
	close(tab.closed)
}



func (tab *Table) doRefresh(done chan struct{}) {
	defer close(done)

	
	
	
	tab.loadSeedNodes()

	
	tab.net.lookupSelf()

	
	
	
	
	
	
	for i := 0; i < 3; i++ {
		tab.net.lookupRandom()
	}
}

func (tab *Table) loadSeedNodes() {
	seeds := wrapNodes(tab.db.QuerySeeds(seedCount, seedMaxAge))
	seeds = append(seeds, tab.nursery...)
	for i := range seeds {
		seed := seeds[i]
		age := log.Lazy{Fn: func() interface{} { return time.Since(tab.db.LastPongReceived(seed.ID(), seed.IP())) }}
		tab.log.Trace("Found seed node in database", "id", seed.ID(), "addr", seed.addr(), "age", age)
		tab.addSeenNode(seed)
	}
}



func (tab *Table) doRevalidate(done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	last, bi := tab.nodeToRevalidate()
	if last == nil {
		
		return
	}

	
	remoteSeq, err := tab.net.ping(unwrapNode(last))

	
	if last.Seq() < remoteSeq {
		n, err := tab.net.RequestENR(unwrapNode(last))
		if err != nil {
			tab.log.Debug("ENR request failed", "id", last.ID(), "addr", last.addr(), "err", err)
		} else {
			last = &node{Node: *n, addedAt: last.addedAt, livenessChecks: last.livenessChecks}
		}
	}

	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	b := tab.buckets[bi]
	if err == nil {
		
		last.livenessChecks++
		tab.log.Debug("Revalidated node", "b", bi, "id", last.ID(), "checks", last.livenessChecks)
		tab.bumpInBucket(b, last)
		return
	}
	
	
	if r := tab.replace(b, last); r != nil {
		tab.log.Debug("Replaced dead node", "b", bi, "id", last.ID(), "ip", last.IP(), "checks", last.livenessChecks, "r", r.ID(), "rip", r.IP())
	} else {
		tab.log.Debug("Removed dead node", "b", bi, "id", last.ID(), "ip", last.IP(), "checks", last.livenessChecks)
	}
}


func (tab *Table) nodeToRevalidate() (n *node, bi int) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	for _, bi = range tab.rand.Perm(len(tab.buckets)) {
		b := tab.buckets[bi]
		if len(b.entries) > 0 {
			last := b.entries[len(b.entries)-1]
			return last, bi
		}
	}
	return nil, 0
}

func (tab *Table) nextRevalidateTime() time.Duration {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	return time.Duration(tab.rand.Int63n(int64(revalidateInterval)))
}



func (tab *Table) copyLiveNodes() {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	now := time.Now()
	for _, b := range &tab.buckets {
		for _, n := range b.entries {
			if n.livenessChecks > 0 && now.Sub(n.addedAt) >= seedMinTableTime {
				tab.db.UpdateNode(unwrapNode(n))
			}
		}
	}
}








func (tab *Table) findnodeByID(target enode.ID, nresults int, preferLive bool) *nodesByDistance {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	
	
	
	nodes := &nodesByDistance{target: target}
	liveNodes := &nodesByDistance{target: target}
	for _, b := range &tab.buckets {
		for _, n := range b.entries {
			nodes.push(n, nresults)
			if preferLive && n.livenessChecks > 0 {
				liveNodes.push(n, nresults)
			}
		}
	}

	if preferLive && len(liveNodes.entries) > 0 {
		return liveNodes
	}
	return nodes
}


func (tab *Table) len() (n int) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	for _, b := range &tab.buckets {
		n += len(b.entries)
	}
	return n
}


func (tab *Table) bucketLen(id enode.ID) int {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	return len(tab.bucket(id).entries)
}


func (tab *Table) bucket(id enode.ID) *bucket {
	d := enode.LogDist(tab.self().ID(), id)
	return tab.bucketAtDistance(d)
}

func (tab *Table) bucketAtDistance(d int) *bucket {
	if d <= bucketMinDistance {
		return tab.buckets[0]
	}
	return tab.buckets[d-bucketMinDistance-1]
}






func (tab *Table) addSeenNode(n *node) {
	if n.ID() == tab.self().ID() {
		return
	}

	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	b := tab.bucket(n.ID())
	if contains(b.entries, n.ID()) {
		
		return
	}
	if len(b.entries) >= bucketSize {
		
		tab.addReplacement(b, n)
		return
	}
	if !tab.addIP(b, n.IP()) {
		
		return
	}
	
	b.entries = append(b.entries, n)
	b.replacements = deleteNode(b.replacements, n)
	n.addedAt = time.Now()
	if tab.nodeAddedHook != nil {
		tab.nodeAddedHook(n)
	}
}










func (tab *Table) addVerifiedNode(n *node) {
	if !tab.isInitDone() {
		return
	}
	if n.ID() == tab.self().ID() {
		return
	}

	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	b := tab.bucket(n.ID())
	if tab.bumpInBucket(b, n) {
		
		return
	}
	if len(b.entries) >= bucketSize {
		
		tab.addReplacement(b, n)
		return
	}
	if !tab.addIP(b, n.IP()) {
		
		return
	}
	
	b.entries, _ = pushNode(b.entries, n, bucketSize)
	b.replacements = deleteNode(b.replacements, n)
	n.addedAt = time.Now()
	if tab.nodeAddedHook != nil {
		tab.nodeAddedHook(n)
	}
}


func (tab *Table) delete(node *node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	tab.deleteInBucket(tab.bucket(node.ID()), node)
}

func (tab *Table) addIP(b *bucket, ip net.IP) bool {
	if len(ip) == 0 {
		return false 
	}
	if netutil.IsLAN(ip) {
		return true
	}
	if !tab.ips.Add(ip) {
		tab.log.Debug("IP exceeds table limit", "ip", ip)
		return false
	}
	if !b.ips.Add(ip) {
		tab.log.Debug("IP exceeds bucket limit", "ip", ip)
		tab.ips.Remove(ip)
		return false
	}
	return true
}

func (tab *Table) removeIP(b *bucket, ip net.IP) {
	if netutil.IsLAN(ip) {
		return
	}
	tab.ips.Remove(ip)
	b.ips.Remove(ip)
}

func (tab *Table) addReplacement(b *bucket, n *node) {
	for _, e := range b.replacements {
		if e.ID() == n.ID() {
			return 
		}
	}
	if !tab.addIP(b, n.IP()) {
		return
	}
	var removed *node
	b.replacements, removed = pushNode(b.replacements, n, maxReplacements)
	if removed != nil {
		tab.removeIP(b, removed.IP())
	}
}




func (tab *Table) replace(b *bucket, last *node) *node {
	if len(b.entries) == 0 || b.entries[len(b.entries)-1].ID() != last.ID() {
		
		return nil
	}
	
	if len(b.replacements) == 0 {
		tab.deleteInBucket(b, last)
		return nil
	}
	r := b.replacements[tab.rand.Intn(len(b.replacements))]
	b.replacements = deleteNode(b.replacements, r)
	b.entries[len(b.entries)-1] = r
	tab.removeIP(b, last.IP())
	return r
}



func (tab *Table) bumpInBucket(b *bucket, n *node) bool {
	for i := range b.entries {
		if b.entries[i].ID() == n.ID() {
			if !n.IP().Equal(b.entries[i].IP()) {
				
				tab.removeIP(b, b.entries[i].IP())
				if !tab.addIP(b, n.IP()) {
					
					tab.addIP(b, b.entries[i].IP())
					return false
				}
			}
			
			copy(b.entries[1:], b.entries[:i])
			b.entries[0] = n
			return true
		}
	}
	return false
}

func (tab *Table) deleteInBucket(b *bucket, n *node) {
	b.entries = deleteNode(b.entries, n)
	tab.removeIP(b, n.IP())
}

func contains(ns []*node, id enode.ID) bool {
	for _, n := range ns {
		if n.ID() == id {
			return true
		}
	}
	return false
}


func pushNode(list []*node, n *node, max int) ([]*node, *node) {
	if len(list) < max {
		list = append(list, nil)
	}
	removed := list[len(list)-1]
	copy(list[1:], list)
	list[0] = n
	return list, removed
}


func deleteNode(list []*node, n *node) []*node {
	for i := range list {
		if list[i].ID() == n.ID() {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}


type nodesByDistance struct {
	entries []*node
	target  enode.ID
}


func (h *nodesByDistance) push(n *node, maxElems int) {
	ix := sort.Search(len(h.entries), func(i int) bool {
		return enode.DistCmp(h.target, h.entries[i].ID(), n.ID()) > 0
	})
	if len(h.entries) < maxElems {
		h.entries = append(h.entries, n)
	}
	if ix == len(h.entries) {
		
		
	} else {
		
		
		copy(h.entries[ix+1:], h.entries[ix:])
		h.entries[ix] = n
	}
}
