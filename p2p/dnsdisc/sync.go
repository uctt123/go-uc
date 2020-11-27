















package dnsdisc

import (
	"context"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

const (
	rootRecheckFailCount = 5 
)


type clientTree struct {
	c   *Client
	loc *linkEntry 

	lastRootCheck mclock.AbsTime 
	leafFailCount int
	rootFailCount int

	root  *rootEntry
	enrs  *subtreeSync
	links *subtreeSync

	lc         *linkCache          
	curLinks   map[string]struct{} 
	linkGCRoot string              
}

func newClientTree(c *Client, lc *linkCache, loc *linkEntry) *clientTree {
	return &clientTree{c: c, lc: lc, loc: loc}
}


func (ct *clientTree) syncAll(dest map[string]entry) error {
	if err := ct.updateRoot(context.Background()); err != nil {
		return err
	}
	if err := ct.links.resolveAll(dest); err != nil {
		return err
	}
	if err := ct.enrs.resolveAll(dest); err != nil {
		return err
	}
	return nil
}



func (ct *clientTree) syncRandom(ctx context.Context) (n *enode.Node, err error) {
	if ct.rootUpdateDue() {
		if err := ct.updateRoot(ctx); err != nil {
			return nil, err
		}
	}

	
	defer func() {
		if err != nil {
			ct.leafFailCount++
		}
	}()

	
	if !ct.links.done() {
		err := ct.syncNextLink(ctx)
		return nil, err
	}
	ct.gcLinks()

	
	
	if ct.enrs.done() {
		ct.enrs = newSubtreeSync(ct.c, ct.loc, ct.root.eroot, false)
	}
	return ct.syncNextRandomENR(ctx)
}



func (ct *clientTree) gcLinks() {
	if !ct.links.done() || ct.root.lroot == ct.linkGCRoot {
		return
	}
	ct.lc.resetLinks(ct.loc.str, ct.curLinks)
	ct.linkGCRoot = ct.root.lroot
}

func (ct *clientTree) syncNextLink(ctx context.Context) error {
	hash := ct.links.missing[0]
	e, err := ct.links.resolveNext(ctx, hash)
	if err != nil {
		return err
	}
	ct.links.missing = ct.links.missing[1:]

	if dest, ok := e.(*linkEntry); ok {
		ct.lc.addLink(ct.loc.str, dest.str)
		ct.curLinks[dest.str] = struct{}{}
	}
	return nil
}

func (ct *clientTree) syncNextRandomENR(ctx context.Context) (*enode.Node, error) {
	index := rand.Intn(len(ct.enrs.missing))
	hash := ct.enrs.missing[index]
	e, err := ct.enrs.resolveNext(ctx, hash)
	if err != nil {
		return nil, err
	}
	ct.enrs.missing = removeHash(ct.enrs.missing, index)
	if ee, ok := e.(*enrEntry); ok {
		return ee.node, nil
	}
	return nil, nil
}

func (ct *clientTree) String() string {
	return ct.loc.String()
}


func removeHash(h []string, index int) []string {
	if len(h) == 1 {
		return nil
	}
	last := len(h) - 1
	if index < last {
		h[index] = h[last]
		h[last] = ""
	}
	return h[:last]
}


func (ct *clientTree) updateRoot(ctx context.Context) error {
	if !ct.slowdownRootUpdate(ctx) {
		return ctx.Err()
	}

	ct.lastRootCheck = ct.c.clock.Now()
	ctx, cancel := context.WithTimeout(ctx, ct.c.cfg.Timeout)
	defer cancel()
	root, err := ct.c.resolveRoot(ctx, ct.loc)
	if err != nil {
		ct.rootFailCount++
		return err
	}
	ct.root = &root
	ct.rootFailCount = 0
	ct.leafFailCount = 0

	
	if ct.links == nil || root.lroot != ct.links.root {
		ct.links = newSubtreeSync(ct.c, ct.loc, root.lroot, true)
		ct.curLinks = make(map[string]struct{})
	}
	if ct.enrs == nil || root.eroot != ct.enrs.root {
		ct.enrs = newSubtreeSync(ct.c, ct.loc, root.eroot, false)
	}
	return nil
}


func (ct *clientTree) rootUpdateDue() bool {
	tooManyFailures := ct.leafFailCount > rootRecheckFailCount
	scheduledCheck := ct.c.clock.Now().Sub(ct.lastRootCheck) > ct.c.cfg.RecheckInterval
	return ct.root == nil || tooManyFailures || scheduledCheck
}




func (ct *clientTree) slowdownRootUpdate(ctx context.Context) bool {
	var delay time.Duration
	switch {
	case ct.rootFailCount > 20:
		delay = 10 * time.Second
	case ct.rootFailCount > 5:
		delay = 5 * time.Second
	default:
		return true
	}
	timeout := ct.c.clock.NewTimer(delay)
	defer timeout.Stop()
	select {
	case <-timeout.C():
		return true
	case <-ctx.Done():
		return false
	}
}


type subtreeSync struct {
	c       *Client
	loc     *linkEntry
	root    string
	missing []string 
	link    bool     
}

func newSubtreeSync(c *Client, loc *linkEntry, root string, link bool) *subtreeSync {
	return &subtreeSync{c, loc, root, []string{root}, link}
}

func (ts *subtreeSync) done() bool {
	return len(ts.missing) == 0
}

func (ts *subtreeSync) resolveAll(dest map[string]entry) error {
	for !ts.done() {
		hash := ts.missing[0]
		ctx, cancel := context.WithTimeout(context.Background(), ts.c.cfg.Timeout)
		e, err := ts.resolveNext(ctx, hash)
		cancel()
		if err != nil {
			return err
		}
		dest[hash] = e
		ts.missing = ts.missing[1:]
	}
	return nil
}

func (ts *subtreeSync) resolveNext(ctx context.Context, hash string) (entry, error) {
	e, err := ts.c.resolveEntry(ctx, ts.loc.domain, hash)
	if err != nil {
		return nil, err
	}
	switch e := e.(type) {
	case *enrEntry:
		if ts.link {
			return nil, errENRInLinkTree
		}
	case *linkEntry:
		if !ts.link {
			return nil, errLinkInENRTree
		}
	case *branchEntry:
		ts.missing = append(ts.missing, e.children...)
	}
	return e, nil
}


type linkCache struct {
	backrefs map[string]map[string]struct{}
	changed  bool
}

func (lc *linkCache) isReferenced(r string) bool {
	return len(lc.backrefs[r]) != 0
}

func (lc *linkCache) addLink(from, to string) {
	if _, ok := lc.backrefs[to][from]; ok {
		return
	}

	if lc.backrefs == nil {
		lc.backrefs = make(map[string]map[string]struct{})
	}
	if _, ok := lc.backrefs[to]; !ok {
		lc.backrefs[to] = make(map[string]struct{})
	}
	lc.backrefs[to][from] = struct{}{}
	lc.changed = true
}


func (lc *linkCache) resetLinks(from string, keep map[string]struct{}) {
	stk := []string{from}
	for len(stk) > 0 {
		item := stk[len(stk)-1]
		stk = stk[:len(stk)-1]

		for r, refs := range lc.backrefs {
			if _, ok := keep[r]; ok {
				continue
			}
			if _, ok := refs[item]; !ok {
				continue
			}
			lc.changed = true
			delete(refs, item)
			if len(refs) == 0 {
				delete(lc.backrefs, r)
				stk = append(stk, r)
			}
		}
	}
}
