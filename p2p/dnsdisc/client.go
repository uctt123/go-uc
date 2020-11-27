















package dnsdisc

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/time/rate"
)


type Client struct {
	cfg     Config
	clock   mclock.Clock
	entries *lru.Cache
}


type Config struct {
	Timeout         time.Duration      
	RecheckInterval time.Duration      
	CacheLimit      int                
	RateLimit       float64            
	ValidSchemes    enr.IdentityScheme 
	Resolver        Resolver           
	Logger          log.Logger         
}


type Resolver interface {
	LookupTXT(ctx context.Context, domain string) ([]string, error)
}

func (cfg Config) withDefaults() Config {
	const (
		defaultTimeout   = 5 * time.Second
		defaultRecheck   = 30 * time.Minute
		defaultRateLimit = 3
		defaultCache     = 1000
	)
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.RecheckInterval == 0 {
		cfg.RecheckInterval = defaultRecheck
	}
	if cfg.CacheLimit == 0 {
		cfg.CacheLimit = defaultCache
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = defaultRateLimit
	}
	if cfg.ValidSchemes == nil {
		cfg.ValidSchemes = enode.ValidSchemes
	}
	if cfg.Resolver == nil {
		cfg.Resolver = new(net.Resolver)
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Root()
	}
	return cfg
}


func NewClient(cfg Config) *Client {
	cfg = cfg.withDefaults()
	cache, err := lru.New(cfg.CacheLimit)
	if err != nil {
		panic(err)
	}
	rlimit := rate.NewLimiter(rate.Limit(cfg.RateLimit), 10)
	cfg.Resolver = &rateLimitResolver{cfg.Resolver, rlimit}
	return &Client{cfg: cfg, entries: cache, clock: mclock.System{}}
}


func (c *Client) SyncTree(url string) (*Tree, error) {
	le, err := parseLink(url)
	if err != nil {
		return nil, fmt.Errorf("invalid enrtree URL: %v", err)
	}
	ct := newClientTree(c, new(linkCache), le)
	t := &Tree{entries: make(map[string]entry)}
	if err := ct.syncAll(t.entries); err != nil {
		return nil, err
	}
	t.root = ct.root
	return t, nil
}



func (c *Client) NewIterator(urls ...string) (enode.Iterator, error) {
	it := c.newRandomIterator()
	for _, url := range urls {
		if err := it.addTree(url); err != nil {
			return nil, err
		}
	}
	return it, nil
}


func (c *Client) resolveRoot(ctx context.Context, loc *linkEntry) (rootEntry, error) {
	txts, err := c.cfg.Resolver.LookupTXT(ctx, loc.domain)
	c.cfg.Logger.Trace("Updating DNS discovery root", "tree", loc.domain, "err", err)
	if err != nil {
		return rootEntry{}, err
	}
	for _, txt := range txts {
		if strings.HasPrefix(txt, rootPrefix) {
			return parseAndVerifyRoot(txt, loc)
		}
	}
	return rootEntry{}, nameError{loc.domain, errNoRoot}
}

func parseAndVerifyRoot(txt string, loc *linkEntry) (rootEntry, error) {
	e, err := parseRoot(txt)
	if err != nil {
		return e, err
	}
	if !e.verifySignature(loc.pubkey) {
		return e, entryError{typ: "root", err: errInvalidSig}
	}
	return e, nil
}



func (c *Client) resolveEntry(ctx context.Context, domain, hash string) (entry, error) {
	cacheKey := truncateHash(hash)
	if e, ok := c.entries.Get(cacheKey); ok {
		return e.(entry), nil
	}
	e, err := c.doResolveEntry(ctx, domain, hash)
	if err != nil {
		return nil, err
	}
	c.entries.Add(cacheKey, e)
	return e, nil
}


func (c *Client) doResolveEntry(ctx context.Context, domain, hash string) (entry, error) {
	wantHash, err := b32format.DecodeString(hash)
	if err != nil {
		return nil, fmt.Errorf("invalid base32 hash")
	}
	name := hash + "." + domain
	txts, err := c.cfg.Resolver.LookupTXT(ctx, hash+"."+domain)
	c.cfg.Logger.Trace("DNS discovery lookup", "name", name, "err", err)
	if err != nil {
		return nil, err
	}
	for _, txt := range txts {
		e, err := parseEntry(txt, c.cfg.ValidSchemes)
		if err == errUnknownEntry {
			continue
		}
		if !bytes.HasPrefix(crypto.Keccak256([]byte(txt)), wantHash) {
			err = nameError{name, errHashMismatch}
		} else if err != nil {
			err = nameError{name, err}
		}
		return e, err
	}
	return nil, nameError{name, errNoEntry}
}


type rateLimitResolver struct {
	r       Resolver
	limiter *rate.Limiter
}

func (r *rateLimitResolver) LookupTXT(ctx context.Context, domain string) ([]string, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return r.r.LookupTXT(ctx, domain)
}


type randomIterator struct {
	cur      *enode.Node
	ctx      context.Context
	cancelFn context.CancelFunc
	c        *Client

	mu    sync.Mutex
	trees map[string]*clientTree 
	lc    linkCache              
}

func (c *Client) newRandomIterator() *randomIterator {
	ctx, cancel := context.WithCancel(context.Background())
	return &randomIterator{
		c:        c,
		ctx:      ctx,
		cancelFn: cancel,
		trees:    make(map[string]*clientTree),
	}
}


func (it *randomIterator) Node() *enode.Node {
	return it.cur
}


func (it *randomIterator) Close() {
	it.mu.Lock()
	defer it.mu.Unlock()

	it.cancelFn()
	it.trees = nil
}


func (it *randomIterator) Next() bool {
	it.cur = it.nextNode()
	return it.cur != nil
}


func (it *randomIterator) addTree(url string) error {
	le, err := parseLink(url)
	if err != nil {
		return fmt.Errorf("invalid enrtree URL: %v", err)
	}
	it.lc.addLink("", le.str)
	return nil
}


func (it *randomIterator) nextNode() *enode.Node {
	for {
		ct := it.nextTree()
		if ct == nil {
			return nil
		}
		n, err := ct.syncRandom(it.ctx)
		if err != nil {
			if err == it.ctx.Err() {
				return nil 
			}
			it.c.cfg.Logger.Debug("Error in DNS random node sync", "tree", ct.loc.domain, "err", err)
			continue
		}
		if n != nil {
			return n
		}
	}
}


func (it *randomIterator) nextTree() *clientTree {
	it.mu.Lock()
	defer it.mu.Unlock()

	if it.lc.changed {
		it.rebuildTrees()
		it.lc.changed = false
	}
	if len(it.trees) == 0 {
		return nil
	}
	limit := rand.Intn(len(it.trees))
	for _, ct := range it.trees {
		if limit == 0 {
			return ct
		}
		limit--
	}
	return nil
}


func (it *randomIterator) rebuildTrees() {
	
	for loc := range it.trees {
		if !it.lc.isReferenced(loc) {
			delete(it.trees, loc)
		}
	}
	
	for loc := range it.lc.backrefs {
		if it.trees[loc] == nil {
			link, _ := parseLink(linkPrefix + loc)
			it.trees[loc] = newClientTree(it.c, &it.lc, link)
		}
	}
}
