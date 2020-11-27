















package state

import (
	"errors"
	"fmt"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	lru "github.com/hashicorp/golang-lru"
)

const (
	
	codeSizeCacheSize = 100000

	
	codeCacheSize = 64 * 1024 * 1024
)


type Database interface {
	
	OpenTrie(root common.Hash) (Trie, error)

	
	OpenStorageTrie(addrHash, root common.Hash) (Trie, error)

	
	CopyTrie(Trie) Trie

	
	ContractCode(addrHash, codeHash common.Hash) ([]byte, error)

	
	ContractCodeSize(addrHash, codeHash common.Hash) (int, error)

	
	TrieDB() *trie.Database
}


type Trie interface {
	
	
	
	
	GetKey([]byte) []byte

	
	
	
	TryGet(key []byte) ([]byte, error)

	
	
	
	
	TryUpdate(key, value []byte) error

	
	
	TryDelete(key []byte) error

	
	
	Hash() common.Hash

	
	
	Commit(onleaf trie.LeafCallback) (common.Hash, error)

	
	
	NodeIterator(startKey []byte) trie.NodeIterator

	
	
	
	
	
	
	
	Prove(key []byte, fromLevel uint, proofDb ethdb.KeyValueWriter) error
}




func NewDatabase(db ethdb.Database) Database {
	return NewDatabaseWithCache(db, 0, "")
}




func NewDatabaseWithCache(db ethdb.Database, cache int, journal string) Database {
	csc, _ := lru.New(codeSizeCacheSize)
	return &cachingDB{
		db:            trie.NewDatabaseWithCache(db, cache, journal),
		codeSizeCache: csc,
		codeCache:     fastcache.New(codeCacheSize),
	}
}

type cachingDB struct {
	db            *trie.Database
	codeSizeCache *lru.Cache
	codeCache     *fastcache.Cache
}


func (db *cachingDB) OpenTrie(root common.Hash) (Trie, error) {
	return trie.NewSecure(root, db.db)
}


func (db *cachingDB) OpenStorageTrie(addrHash, root common.Hash) (Trie, error) {
	return trie.NewSecure(root, db.db)
}


func (db *cachingDB) CopyTrie(t Trie) Trie {
	switch t := t.(type) {
	case *trie.SecureTrie:
		return t.Copy()
	default:
		panic(fmt.Errorf("unknown trie type %T", t))
	}
}


func (db *cachingDB) ContractCode(addrHash, codeHash common.Hash) ([]byte, error) {
	if code := db.codeCache.Get(nil, codeHash.Bytes()); len(code) > 0 {
		return code, nil
	}
	code := rawdb.ReadCode(db.db.DiskDB(), codeHash)
	if len(code) > 0 {
		db.codeCache.Set(codeHash.Bytes(), code)
		db.codeSizeCache.Add(codeHash, len(code))
		return code, nil
	}
	return nil, errors.New("not found")
}




func (db *cachingDB) ContractCodeWithPrefix(addrHash, codeHash common.Hash) ([]byte, error) {
	if code := db.codeCache.Get(nil, codeHash.Bytes()); len(code) > 0 {
		return code, nil
	}
	code := rawdb.ReadCodeWithPrefix(db.db.DiskDB(), codeHash)
	if len(code) > 0 {
		db.codeCache.Set(codeHash.Bytes(), code)
		db.codeSizeCache.Add(codeHash, len(code))
		return code, nil
	}
	return nil, errors.New("not found")
}


func (db *cachingDB) ContractCodeSize(addrHash, codeHash common.Hash) (int, error) {
	if cached, ok := db.codeSizeCache.Get(codeHash); ok {
		return cached.(int), nil
	}
	code, err := db.ContractCode(addrHash, codeHash)
	return len(code), err
}


func (db *cachingDB) TrieDB() *trie.Database {
	return db.db
}
