















package v5wire

import (
	"crypto/ecdsa"
	crand "crypto/rand"
	"encoding/binary"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/hashicorp/golang-lru/simplelru"
)

const handshakeTimeout = time.Second



type SessionCache struct {
	sessions   *simplelru.LRU
	handshakes map[sessionID]*Whoareyou
	clock      mclock.Clock

	
	nonceGen        func(uint32) (Nonce, error)
	maskingIVGen    func([]byte) error
	ephemeralKeyGen func() (*ecdsa.PrivateKey, error)
}


type sessionID struct {
	id   enode.ID
	addr string
}


type session struct {
	writeKey     []byte
	readKey      []byte
	nonceCounter uint32
}


func (s *session) keysFlipped() *session {
	return &session{s.readKey, s.writeKey, s.nonceCounter}
}

func NewSessionCache(maxItems int, clock mclock.Clock) *SessionCache {
	cache, err := simplelru.NewLRU(maxItems, nil)
	if err != nil {
		panic("can't create session cache")
	}
	return &SessionCache{
		sessions:        cache,
		handshakes:      make(map[sessionID]*Whoareyou),
		clock:           clock,
		nonceGen:        generateNonce,
		maskingIVGen:    generateMaskingIV,
		ephemeralKeyGen: crypto.GenerateKey,
	}
}

func generateNonce(counter uint32) (n Nonce, err error) {
	binary.BigEndian.PutUint32(n[:4], counter)
	_, err = crand.Read(n[4:])
	return n, err
}

func generateMaskingIV(buf []byte) error {
	_, err := crand.Read(buf)
	return err
}


func (sc *SessionCache) nextNonce(s *session) (Nonce, error) {
	s.nonceCounter++
	return sc.nonceGen(s.nonceCounter)
}


func (sc *SessionCache) session(id enode.ID, addr string) *session {
	item, ok := sc.sessions.Get(sessionID{id, addr})
	if !ok {
		return nil
	}
	return item.(*session)
}


func (sc *SessionCache) readKey(id enode.ID, addr string) []byte {
	if s := sc.session(id, addr); s != nil {
		return s.readKey
	}
	return nil
}


func (sc *SessionCache) storeNewSession(id enode.ID, addr string, s *session) {
	sc.sessions.Add(sessionID{id, addr}, s)
}


func (sc *SessionCache) getHandshake(id enode.ID, addr string) *Whoareyou {
	return sc.handshakes[sessionID{id, addr}]
}


func (sc *SessionCache) storeSentHandshake(id enode.ID, addr string, challenge *Whoareyou) {
	challenge.sent = sc.clock.Now()
	sc.handshakes[sessionID{id, addr}] = challenge
}


func (sc *SessionCache) deleteHandshake(id enode.ID, addr string) {
	delete(sc.handshakes, sessionID{id, addr})
}


func (sc *SessionCache) handshakeGC() {
	deadline := sc.clock.Now().Add(-handshakeTimeout)
	for key, challenge := range sc.handshakes {
		if challenge.sent < deadline {
			delete(sc.handshakes, key)
		}
	}
}
