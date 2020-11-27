















package v5test

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/discover/v5wire"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
)



type readError struct {
	err error
}

func (p *readError) Kind() byte          { return 99 }
func (p *readError) Name() string        { return fmt.Sprintf("error: %v", p.err) }
func (p *readError) Error() string       { return p.err.Error() }
func (p *readError) Unwrap() error       { return p.err }
func (p *readError) RequestID() []byte   { return nil }
func (p *readError) SetRequestID([]byte) {}


func readErrorf(format string, args ...interface{}) *readError {
	return &readError{fmt.Errorf(format, args...)}
}


const waitTime = 300 * time.Millisecond


type conn struct {
	localNode  *enode.LocalNode
	localKey   *ecdsa.PrivateKey
	remote     *enode.Node
	remoteAddr *net.UDPAddr
	listeners  []net.PacketConn

	log           logger
	codec         *v5wire.Codec
	lastRequest   v5wire.Packet
	lastChallenge *v5wire.Whoareyou
	idCounter     uint32
}

type logger interface {
	Logf(string, ...interface{})
}


func newConn(dest *enode.Node, log logger) *conn {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	db, err := enode.OpenDB("")
	if err != nil {
		panic(err)
	}
	ln := enode.NewLocalNode(db, key)

	return &conn{
		localKey:   key,
		localNode:  ln,
		remote:     dest,
		remoteAddr: &net.UDPAddr{IP: dest.IP(), Port: dest.UDP()},
		codec:      v5wire.NewCodec(ln, key, mclock.System{}),
		log:        log,
	}
}

func (tc *conn) setEndpoint(c net.PacketConn) {
	tc.localNode.SetStaticIP(laddr(c).IP)
	tc.localNode.SetFallbackUDP(laddr(c).Port)
}

func (tc *conn) listen(ip string) net.PacketConn {
	l, err := net.ListenPacket("udp", fmt.Sprintf("%v:0", ip))
	if err != nil {
		panic(err)
	}
	tc.listeners = append(tc.listeners, l)
	return l
}


func (tc *conn) close() {
	for _, l := range tc.listeners {
		l.Close()
	}
	tc.localNode.Database().Close()
}


func (tc *conn) nextReqID() []byte {
	id := make([]byte, 4)
	tc.idCounter++
	binary.BigEndian.PutUint32(id, tc.idCounter)
	return id
}



func (tc *conn) reqresp(c net.PacketConn, req v5wire.Packet) v5wire.Packet {
	reqnonce := tc.write(c, req, nil)
	switch resp := tc.read(c).(type) {
	case *v5wire.Whoareyou:
		if resp.Nonce != reqnonce {
			return readErrorf("wrong nonce %x in WHOAREYOU (want %x)", resp.Nonce[:], reqnonce[:])
		}
		resp.Node = tc.remote
		tc.write(c, req, resp)
		return tc.read(c)
	default:
		return resp
	}
}


func (tc *conn) findnode(c net.PacketConn, dists []uint) ([]*enode.Node, error) {
	var (
		findnode = &v5wire.Findnode{ReqID: tc.nextReqID(), Distances: dists}
		reqnonce = tc.write(c, findnode, nil)
		first    = true
		total    uint8
		results  []*enode.Node
	)
	for n := 1; n > 0; {
		switch resp := tc.read(c).(type) {
		case *v5wire.Whoareyou:
			
			if resp.Nonce == reqnonce {
				resp.Node = tc.remote
				tc.write(c, findnode, resp)
			} else {
				return nil, fmt.Errorf("unexpected WHOAREYOU (nonce %x), waiting for NODES", resp.Nonce[:])
			}
		case *v5wire.Ping:
			
			tc.write(c, &v5wire.Pong{
				ReqID:  resp.ReqID,
				ENRSeq: tc.localNode.Seq(),
			}, nil)
		case *v5wire.Nodes:
			
			if !bytes.Equal(resp.ReqID, findnode.ReqID) {
				return nil, fmt.Errorf("NODES response has wrong request id %x", resp.ReqID)
			}
			
			
			if first {
				if resp.Total == 0 || resp.Total > 6 {
					return nil, fmt.Errorf("invalid NODES response 'total' %d (not in (0,7))", resp.Total)
				}
				total = resp.Total
				n = int(total) - 1
				first = false
			} else {
				n--
				if resp.Total != total {
					return nil, fmt.Errorf("invalid NODES response 'total' %d (!= %d)", resp.Total, total)
				}
			}
			
			nodes, err := checkRecords(resp.Nodes)
			if err != nil {
				return nil, fmt.Errorf("invalid node in NODES response: %v", err)
			}
			results = append(results, nodes...)
		default:
			return nil, fmt.Errorf("expected NODES, got %v", resp)
		}
	}
	return results, nil
}


func (tc *conn) write(c net.PacketConn, p v5wire.Packet, challenge *v5wire.Whoareyou) v5wire.Nonce {
	packet, nonce, err := tc.codec.Encode(tc.remote.ID(), tc.remoteAddr.String(), p, challenge)
	if err != nil {
		panic(fmt.Errorf("can't encode %v packet: %v", p.Name(), err))
	}
	if _, err := c.WriteTo(packet, tc.remoteAddr); err != nil {
		tc.logf("Can't send %s: %v", p.Name(), err)
	} else {
		tc.logf(">> %s", p.Name())
	}
	return nonce
}


func (tc *conn) read(c net.PacketConn) v5wire.Packet {
	buf := make([]byte, 1280)
	if err := c.SetReadDeadline(time.Now().Add(waitTime)); err != nil {
		return &readError{err}
	}
	n, fromAddr, err := c.ReadFrom(buf)
	if err != nil {
		return &readError{err}
	}
	_, _, p, err := tc.codec.Decode(buf[:n], fromAddr.String())
	if err != nil {
		return &readError{err}
	}
	tc.logf("<< %s", p.Name())
	return p
}


func (tc *conn) logf(format string, args ...interface{}) {
	if tc.log != nil {
		tc.log.Logf("(%s) %s", tc.localNode.ID().TerminalString(), fmt.Sprintf(format, args...))
	}
}

func laddr(c net.PacketConn) *net.UDPAddr {
	return c.LocalAddr().(*net.UDPAddr)
}

func checkRecords(records []*enr.Record) ([]*enode.Node, error) {
	nodes := make([]*enode.Node, len(records))
	for i := range records {
		n, err := enode.New(enode.ValidSchemes, records[i])
		if err != nil {
			return nil, err
		}
		nodes[i] = n
	}
	return nodes, nil
}

func containsUint(ints []uint, x uint) bool {
	for i := range ints {
		if ints[i] == x {
			return true
		}
	}
	return false
}
