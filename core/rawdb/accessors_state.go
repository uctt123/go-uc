















package rawdb

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
)


func ReadPreimage(db ethdb.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(preimageKey(hash))
	return data
}


func WritePreimages(db ethdb.KeyValueWriter, preimages map[common.Hash][]byte) {
	for hash, preimage := range preimages {
		if err := db.Put(preimageKey(hash), preimage); err != nil {
			log.Crit("Failed to store trie preimage", "err", err)
		}
	}
	preimageCounter.Inc(int64(len(preimages)))
	preimageHitCounter.Inc(int64(len(preimages)))
}


func ReadCode(db ethdb.KeyValueReader, hash common.Hash) []byte {
	
	
	
	
	
	data, _ := db.Get(hash[:])
	if len(data) != 0 {
		return data
	}
	return ReadCodeWithPrefix(db, hash)
}




func ReadCodeWithPrefix(db ethdb.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(codeKey(hash))
	return data
}


func WriteCode(db ethdb.KeyValueWriter, hash common.Hash, code []byte) {
	if err := db.Put(codeKey(hash), code); err != nil {
		log.Crit("Failed to store contract code", "err", err)
	}
}


func DeleteCode(db ethdb.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(codeKey(hash)); err != nil {
		log.Crit("Failed to delete contract code", "err", err)
	}
}


func ReadTrieNode(db ethdb.KeyValueReader, hash common.Hash) []byte {
	data, _ := db.Get(hash.Bytes())
	return data
}


func WriteTrieNode(db ethdb.KeyValueWriter, hash common.Hash, node []byte) {
	if err := db.Put(hash.Bytes(), node); err != nil {
		log.Crit("Failed to store trie node", "err", err)
	}
}


func DeleteTrieNode(db ethdb.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(hash.Bytes()); err != nil {
		log.Crit("Failed to delete trie node", "err", err)
	}
}
