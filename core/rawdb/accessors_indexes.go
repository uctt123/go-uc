















package rawdb

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)



func ReadTxLookupEntry(db ethdb.Reader, hash common.Hash) *uint64 {
	data, _ := db.Get(txLookupKey(hash))
	if len(data) == 0 {
		return nil
	}
	
	if len(data) < common.HashLength {
		number := new(big.Int).SetBytes(data).Uint64()
		return &number
	}
	
	if len(data) == common.HashLength {
		return ReadHeaderNumber(db, common.BytesToHash(data))
	}
	
	var entry LegacyTxLookupEntry
	if err := rlp.DecodeBytes(data, &entry); err != nil {
		log.Error("Invalid transaction lookup entry RLP", "hash", hash, "blob", data, "err", err)
		return nil
	}
	return &entry.BlockIndex
}



func writeTxLookupEntry(db ethdb.KeyValueWriter, hash common.Hash, numberBytes []byte) {
	if err := db.Put(txLookupKey(hash), numberBytes); err != nil {
		log.Crit("Failed to store transaction lookup entry", "err", err)
	}
}



func WriteTxLookupEntries(db ethdb.KeyValueWriter, number uint64, hashes []common.Hash) {
	numberBytes := new(big.Int).SetUint64(number).Bytes()
	for _, hash := range hashes {
		writeTxLookupEntry(db, hash, numberBytes)
	}
}



func WriteTxLookupEntriesByBlock(db ethdb.KeyValueWriter, block *types.Block) {
	numberBytes := block.Number().Bytes()
	for _, tx := range block.Transactions() {
		writeTxLookupEntry(db, tx.Hash(), numberBytes)
	}
}


func DeleteTxLookupEntry(db ethdb.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(txLookupKey(hash)); err != nil {
		log.Crit("Failed to delete transaction lookup entry", "err", err)
	}
}


func DeleteTxLookupEntries(db ethdb.KeyValueWriter, hashes []common.Hash) {
	for _, hash := range hashes {
		DeleteTxLookupEntry(db, hash)
	}
}



func ReadTransaction(db ethdb.Reader, hash common.Hash) (*types.Transaction, common.Hash, uint64, uint64) {
	blockNumber := ReadTxLookupEntry(db, hash)
	if blockNumber == nil {
		return nil, common.Hash{}, 0, 0
	}
	blockHash := ReadCanonicalHash(db, *blockNumber)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}
	body := ReadBody(db, blockHash, *blockNumber)
	if body == nil {
		log.Error("Transaction referenced missing", "number", blockNumber, "hash", blockHash)
		return nil, common.Hash{}, 0, 0
	}
	for txIndex, tx := range body.Transactions {
		if tx.Hash() == hash {
			return tx, blockHash, *blockNumber, uint64(txIndex)
		}
	}
	log.Error("Transaction not found", "number", blockNumber, "hash", blockHash, "txhash", hash)
	return nil, common.Hash{}, 0, 0
}



func ReadReceipt(db ethdb.Reader, hash common.Hash, config *params.ChainConfig) (*types.Receipt, common.Hash, uint64, uint64) {
	
	blockNumber := ReadTxLookupEntry(db, hash)
	if blockNumber == nil {
		return nil, common.Hash{}, 0, 0
	}
	blockHash := ReadCanonicalHash(db, *blockNumber)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}
	
	receipts := ReadReceipts(db, blockHash, *blockNumber, config)
	for receiptIndex, receipt := range receipts {
		if receipt.TxHash == hash {
			return receipt, blockHash, *blockNumber, uint64(receiptIndex)
		}
	}
	log.Error("Receipt not found", "number", blockNumber, "hash", blockHash, "txhash", hash)
	return nil, common.Hash{}, 0, 0
}



func ReadBloomBits(db ethdb.KeyValueReader, bit uint, section uint64, head common.Hash) ([]byte, error) {
	return db.Get(bloomBitsKey(bit, section, head))
}



func WriteBloomBits(db ethdb.KeyValueWriter, bit uint, section uint64, head common.Hash, bits []byte) {
	if err := db.Put(bloomBitsKey(bit, section, head), bits); err != nil {
		log.Crit("Failed to store bloom bits", "err", err)
	}
}



func DeleteBloombits(db ethdb.Database, bit uint, from uint64, to uint64) {
	start, end := bloomBitsKey(bit, from, common.Hash{}), bloomBitsKey(bit, to, common.Hash{})
	it := db.NewIterator(nil, start)
	defer it.Release()

	for it.Next() {
		if bytes.Compare(it.Key(), end) >= 0 {
			break
		}
		if len(it.Key()) != len(bloomBitsPrefix)+2+8+32 {
			continue
		}
		db.Delete(it.Key())
	}
	if it.Error() != nil {
		log.Crit("Failed to delete bloom bits", "err", it.Error())
	}
}
