















package enode

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)


const (
	dbVersionKey   = "version" 
	dbNodePrefix   = "n:"      
	dbLocalPrefix  = "local:"
	dbDiscoverRoot = "v4"
	dbDiscv5Root   = "v5"

	
	
	dbNodeFindFails = "findfail"
	dbNodePing      = "lastping"
	dbNodePong      = "lastpong"
	dbNodeSeq       = "seq"

	
	
	dbLocalSeq = "seq"
)

const (
	dbNodeExpiration = 24 * time.Hour 
	dbCleanupCycle   = time.Hour      
	dbVersion        = 9
)

var zeroIP = make(net.IP, 16)



type DB struct {
	lvl    *leveldb.DB   
	runner sync.Once     
	quit   chan struct{} 
}



func OpenDB(path string) (*DB, error) {
	if path == "" {
		return newMemoryDB()
	}
	return newPersistentDB(path)
}


func newMemoryDB() (*DB, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return &DB{lvl: db, quit: make(chan struct{})}, nil
}



func newPersistentDB(path string) (*DB, error) {
	opts := &opt.Options{OpenFilesCacheCapacity: 5}
	db, err := leveldb.OpenFile(path, opts)
	if _, iscorrupted := err.(*errors.ErrCorrupted); iscorrupted {
		db, err = leveldb.RecoverFile(path, nil)
	}
	if err != nil {
		return nil, err
	}
	
	
	currentVer := make([]byte, binary.MaxVarintLen64)
	currentVer = currentVer[:binary.PutVarint(currentVer, int64(dbVersion))]

	blob, err := db.Get([]byte(dbVersionKey), nil)
	switch err {
	case leveldb.ErrNotFound:
		
		if err := db.Put([]byte(dbVersionKey), currentVer, nil); err != nil {
			db.Close()
			return nil, err
		}

	case nil:
		
		if !bytes.Equal(blob, currentVer) {
			db.Close()
			if err = os.RemoveAll(path); err != nil {
				return nil, err
			}
			return newPersistentDB(path)
		}
	}
	return &DB{lvl: db, quit: make(chan struct{})}, nil
}


func nodeKey(id ID) []byte {
	key := append([]byte(dbNodePrefix), id[:]...)
	key = append(key, ':')
	key = append(key, dbDiscoverRoot...)
	return key
}


func splitNodeKey(key []byte) (id ID, rest []byte) {
	if !bytes.HasPrefix(key, []byte(dbNodePrefix)) {
		return ID{}, nil
	}
	item := key[len(dbNodePrefix):]
	copy(id[:], item[:len(id)])
	return id, item[len(id)+1:]
}


func nodeItemKey(id ID, ip net.IP, field string) []byte {
	ip16 := ip.To16()
	if ip16 == nil {
		panic(fmt.Errorf("invalid IP (length %d)", len(ip)))
	}
	return bytes.Join([][]byte{nodeKey(id), ip16, []byte(field)}, []byte{':'})
}


func splitNodeItemKey(key []byte) (id ID, ip net.IP, field string) {
	id, key = splitNodeKey(key)
	
	if string(key) == dbDiscoverRoot {
		return id, nil, ""
	}
	key = key[len(dbDiscoverRoot)+1:]
	
	ip = net.IP(key[:16])
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	key = key[16+1:]
	
	field = string(key)
	return id, ip, field
}

func v5Key(id ID, ip net.IP, field string) []byte {
	return bytes.Join([][]byte{
		[]byte(dbNodePrefix),
		id[:],
		[]byte(dbDiscv5Root),
		ip.To16(),
		[]byte(field),
	}, []byte{':'})
}


func localItemKey(id ID, field string) []byte {
	key := append([]byte(dbLocalPrefix), id[:]...)
	key = append(key, ':')
	key = append(key, field...)
	return key
}


func (db *DB) fetchInt64(key []byte) int64 {
	blob, err := db.lvl.Get(key, nil)
	if err != nil {
		return 0
	}
	val, read := binary.Varint(blob)
	if read <= 0 {
		return 0
	}
	return val
}


func (db *DB) storeInt64(key []byte, n int64) error {
	blob := make([]byte, binary.MaxVarintLen64)
	blob = blob[:binary.PutVarint(blob, n)]
	return db.lvl.Put(key, blob, nil)
}


func (db *DB) fetchUint64(key []byte) uint64 {
	blob, err := db.lvl.Get(key, nil)
	if err != nil {
		return 0
	}
	val, _ := binary.Uvarint(blob)
	return val
}


func (db *DB) storeUint64(key []byte, n uint64) error {
	blob := make([]byte, binary.MaxVarintLen64)
	blob = blob[:binary.PutUvarint(blob, n)]
	return db.lvl.Put(key, blob, nil)
}


func (db *DB) Node(id ID) *Node {
	blob, err := db.lvl.Get(nodeKey(id), nil)
	if err != nil {
		return nil
	}
	return mustDecodeNode(id[:], blob)
}

func mustDecodeNode(id, data []byte) *Node {
	node := new(Node)
	if err := rlp.DecodeBytes(data, &node.r); err != nil {
		panic(fmt.Errorf("p2p/enode: can't decode node %x in DB: %v", id, err))
	}
	
	copy(node.id[:], id)
	return node
}


func (db *DB) UpdateNode(node *Node) error {
	if node.Seq() < db.NodeSeq(node.ID()) {
		return nil
	}
	blob, err := rlp.EncodeToBytes(&node.r)
	if err != nil {
		return err
	}
	if err := db.lvl.Put(nodeKey(node.ID()), blob, nil); err != nil {
		return err
	}
	return db.storeUint64(nodeItemKey(node.ID(), zeroIP, dbNodeSeq), node.Seq())
}


func (db *DB) NodeSeq(id ID) uint64 {
	return db.fetchUint64(nodeItemKey(id, zeroIP, dbNodeSeq))
}



func (db *DB) Resolve(n *Node) *Node {
	if n.Seq() > db.NodeSeq(n.ID()) {
		return n
	}
	return db.Node(n.ID())
}


func (db *DB) DeleteNode(id ID) {
	deleteRange(db.lvl, nodeKey(id))
}

func deleteRange(db *leveldb.DB, prefix []byte) {
	it := db.NewIterator(util.BytesPrefix(prefix), nil)
	defer it.Release()
	for it.Next() {
		db.Delete(it.Key(), nil)
	}
}










func (db *DB) ensureExpirer() {
	db.runner.Do(func() { go db.expirer() })
}



func (db *DB) expirer() {
	tick := time.NewTicker(dbCleanupCycle)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			db.expireNodes()
		case <-db.quit:
			return
		}
	}
}



func (db *DB) expireNodes() {
	it := db.lvl.NewIterator(util.BytesPrefix([]byte(dbNodePrefix)), nil)
	defer it.Release()
	if !it.Next() {
		return
	}

	var (
		threshold    = time.Now().Add(-dbNodeExpiration).Unix()
		youngestPong int64
		atEnd        = false
	)
	for !atEnd {
		id, ip, field := splitNodeItemKey(it.Key())
		if field == dbNodePong {
			time, _ := binary.Varint(it.Value())
			if time > youngestPong {
				youngestPong = time
			}
			if time < threshold {
				
				deleteRange(db.lvl, nodeItemKey(id, ip, ""))
			}
		}
		atEnd = !it.Next()
		nextID, _ := splitNodeKey(it.Key())
		if atEnd || nextID != id {
			
			
			if youngestPong > 0 && youngestPong < threshold {
				deleteRange(db.lvl, nodeKey(id))
			}
			youngestPong = 0
		}
	}
}



func (db *DB) LastPingReceived(id ID, ip net.IP) time.Time {
	return time.Unix(db.fetchInt64(nodeItemKey(id, ip, dbNodePing)), 0)
}


func (db *DB) UpdateLastPingReceived(id ID, ip net.IP, instance time.Time) error {
	return db.storeInt64(nodeItemKey(id, ip, dbNodePing), instance.Unix())
}


func (db *DB) LastPongReceived(id ID, ip net.IP) time.Time {
	
	db.ensureExpirer()
	return time.Unix(db.fetchInt64(nodeItemKey(id, ip, dbNodePong)), 0)
}


func (db *DB) UpdateLastPongReceived(id ID, ip net.IP, instance time.Time) error {
	return db.storeInt64(nodeItemKey(id, ip, dbNodePong), instance.Unix())
}


func (db *DB) FindFails(id ID, ip net.IP) int {
	return int(db.fetchInt64(nodeItemKey(id, ip, dbNodeFindFails)))
}


func (db *DB) UpdateFindFails(id ID, ip net.IP, fails int) error {
	return db.storeInt64(nodeItemKey(id, ip, dbNodeFindFails), int64(fails))
}


func (db *DB) FindFailsV5(id ID, ip net.IP) int {
	return int(db.fetchInt64(v5Key(id, ip, dbNodeFindFails)))
}


func (db *DB) UpdateFindFailsV5(id ID, ip net.IP, fails int) error {
	return db.storeInt64(v5Key(id, ip, dbNodeFindFails), int64(fails))
}


func (db *DB) localSeq(id ID) uint64 {
	return db.fetchUint64(localItemKey(id, dbLocalSeq))
}


func (db *DB) storeLocalSeq(id ID, n uint64) {
	db.storeUint64(localItemKey(id, dbLocalSeq), n)
}



func (db *DB) QuerySeeds(n int, maxAge time.Duration) []*Node {
	var (
		now   = time.Now()
		nodes = make([]*Node, 0, n)
		it    = db.lvl.NewIterator(nil, nil)
		id    ID
	)
	defer it.Release()

seek:
	for seeks := 0; len(nodes) < n && seeks < n*5; seeks++ {
		
		
		
		ctr := id[0]
		rand.Read(id[:])
		id[0] = ctr + id[0]%16
		it.Seek(nodeKey(id))

		n := nextNode(it)
		if n == nil {
			id[0] = 0
			continue seek 
		}
		if now.Sub(db.LastPongReceived(n.ID(), n.IP())) > maxAge {
			continue seek
		}
		for i := range nodes {
			if nodes[i].ID() == n.ID() {
				continue seek 
			}
		}
		nodes = append(nodes, n)
	}
	return nodes
}



func nextNode(it iterator.Iterator) *Node {
	for end := false; !end; end = !it.Next() {
		id, rest := splitNodeKey(it.Key())
		if string(rest) != dbDiscoverRoot {
			continue
		}
		return mustDecodeNode(id[:], it.Value())
	}
	return nil
}


func (db *DB) Close() {
	close(db.quit)
	db.lvl.Close()
}
