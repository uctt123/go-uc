















package enode

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)

const (
	
	iptrackMinStatements = 10
	iptrackWindow        = 5 * time.Minute
	iptrackContactWindow = 10 * time.Minute
)




type LocalNode struct {
	cur atomic.Value 
	id  ID
	key *ecdsa.PrivateKey
	db  *DB

	
	mu        sync.Mutex
	seq       uint64
	entries   map[string]enr.Entry
	endpoint4 lnEndpoint
	endpoint6 lnEndpoint
}

type lnEndpoint struct {
	track                *netutil.IPTracker
	staticIP, fallbackIP net.IP
	fallbackUDP          int
}


func NewLocalNode(db *DB, key *ecdsa.PrivateKey) *LocalNode {
	ln := &LocalNode{
		id:      PubkeyToIDV4(&key.PublicKey),
		db:      db,
		key:     key,
		entries: make(map[string]enr.Entry),
		endpoint4: lnEndpoint{
			track: netutil.NewIPTracker(iptrackWindow, iptrackContactWindow, iptrackMinStatements),
		},
		endpoint6: lnEndpoint{
			track: netutil.NewIPTracker(iptrackWindow, iptrackContactWindow, iptrackMinStatements),
		},
	}
	ln.seq = db.localSeq(ln.id)
	ln.invalidate()
	return ln
}


func (ln *LocalNode) Database() *DB {
	return ln.db
}


func (ln *LocalNode) Node() *Node {
	n := ln.cur.Load().(*Node)
	if n != nil {
		return n
	}
	
	ln.mu.Lock()
	defer ln.mu.Unlock()
	ln.sign()
	return ln.cur.Load().(*Node)
}


func (ln *LocalNode) Seq() uint64 {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	return ln.seq
}


func (ln *LocalNode) ID() ID {
	return ln.id
}




func (ln *LocalNode) Set(e enr.Entry) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.set(e)
}

func (ln *LocalNode) set(e enr.Entry) {
	val, exists := ln.entries[e.ENRKey()]
	if !exists || !reflect.DeepEqual(val, e) {
		ln.entries[e.ENRKey()] = e
		ln.invalidate()
	}
}


func (ln *LocalNode) Delete(e enr.Entry) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.delete(e)
}

func (ln *LocalNode) delete(e enr.Entry) {
	_, exists := ln.entries[e.ENRKey()]
	if exists {
		delete(ln.entries, e.ENRKey())
		ln.invalidate()
	}
}

func (ln *LocalNode) endpointForIP(ip net.IP) *lnEndpoint {
	if ip.To4() != nil {
		return &ln.endpoint4
	}
	return &ln.endpoint6
}



func (ln *LocalNode) SetStaticIP(ip net.IP) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.endpointForIP(ip).staticIP = ip
	ln.updateEndpoints()
}



func (ln *LocalNode) SetFallbackIP(ip net.IP) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.endpointForIP(ip).fallbackIP = ip
	ln.updateEndpoints()
}



func (ln *LocalNode) SetFallbackUDP(port int) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.endpoint4.fallbackUDP = port
	ln.endpoint6.fallbackUDP = port
	ln.updateEndpoints()
}



func (ln *LocalNode) UDPEndpointStatement(fromaddr, endpoint *net.UDPAddr) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.endpointForIP(endpoint.IP).track.AddStatement(fromaddr.String(), endpoint.String())
	ln.updateEndpoints()
}



func (ln *LocalNode) UDPContact(toaddr *net.UDPAddr) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.endpointForIP(toaddr.IP).track.AddContact(toaddr.String())
	ln.updateEndpoints()
}


func (ln *LocalNode) updateEndpoints() {
	ip4, udp4 := ln.endpoint4.get()
	ip6, udp6 := ln.endpoint6.get()

	if ip4 != nil && !ip4.IsUnspecified() {
		ln.set(enr.IPv4(ip4))
	} else {
		ln.delete(enr.IPv4{})
	}
	if ip6 != nil && !ip6.IsUnspecified() {
		ln.set(enr.IPv6(ip6))
	} else {
		ln.delete(enr.IPv6{})
	}
	if udp4 != 0 {
		ln.set(enr.UDP(udp4))
	} else {
		ln.delete(enr.UDP(0))
	}
	if udp6 != 0 && udp6 != udp4 {
		ln.set(enr.UDP6(udp6))
	} else {
		ln.delete(enr.UDP6(0))
	}
}


func (e *lnEndpoint) get() (newIP net.IP, newPort int) {
	newPort = e.fallbackUDP
	if e.fallbackIP != nil {
		newIP = e.fallbackIP
	}
	if e.staticIP != nil {
		newIP = e.staticIP
	} else if ip, port := predictAddr(e.track); ip != nil {
		newIP = ip
		newPort = port
	}
	return newIP, newPort
}



func predictAddr(t *netutil.IPTracker) (net.IP, int) {
	ep := t.PredictEndpoint()
	if ep == "" {
		return nil, 0
	}
	ipString, portString, _ := net.SplitHostPort(ep)
	ip := net.ParseIP(ipString)
	port, _ := strconv.Atoi(portString)
	return ip, port
}

func (ln *LocalNode) invalidate() {
	ln.cur.Store((*Node)(nil))
}

func (ln *LocalNode) sign() {
	if n := ln.cur.Load().(*Node); n != nil {
		return 
	}

	var r enr.Record
	for _, e := range ln.entries {
		r.Set(e)
	}
	ln.bumpSeq()
	r.SetSeq(ln.seq)
	if err := SignV4(&r, ln.key); err != nil {
		panic(fmt.Errorf("enode: can't sign record: %v", err))
	}
	n, err := New(ValidSchemes, &r)
	if err != nil {
		panic(fmt.Errorf("enode: can't verify local record: %v", err))
	}
	ln.cur.Store(n)
	log.Info("New local node record", "seq", ln.seq, "id", n.ID(), "ip", n.IP(), "udp", n.UDP(), "tcp", n.TCP())
}

func (ln *LocalNode) bumpSeq() {
	ln.seq++
	ln.db.storeLocalSeq(ln.id, ln.seq)
}
