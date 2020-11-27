















package light

import (
	"bytes"
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var sha3Nil = crypto.Keccak256Hash(nil)



func GetHeaderByNumber(ctx context.Context, odr OdrBackend, number uint64) (*types.Header, error) {
	
	db := odr.Database()
	hash := rawdb.ReadCanonicalHash(db, number)

	
	
	if (hash != common.Hash{}) {
		if header := rawdb.ReadHeader(db, hash, number); header != nil {
			return header, nil
		}
	}
	
	
	chts, _, chtHead := odr.ChtIndexer().Sections()
	if number >= chts*odr.IndexerConfig().ChtSize {
		return nil, errNoTrustedCht
	}
	r := &ChtRequest{
		ChtRoot:  GetChtRoot(db, chts-1, chtHead),
		ChtNum:   chts - 1,
		BlockNum: number,
		Config:   odr.IndexerConfig(),
	}
	if err := odr.Retrieve(ctx, r); err != nil {
		return nil, err
	}
	return r.Header, nil
}




func GetUntrustedHeaderByNumber(ctx context.Context, odr OdrBackend, number uint64, peerId string) (*types.Header, error) {
	
	
	r := &ChtRequest{
		BlockNum:  number,
		ChtNum:    number / odr.IndexerConfig().ChtSize,
		Untrusted: true,
		PeerId:    peerId,
		Config:    odr.IndexerConfig(),
	}
	if err := odr.Retrieve(ctx, r); err != nil {
		return nil, err
	}
	return r.Header, nil
}


func GetCanonicalHash(ctx context.Context, odr OdrBackend, number uint64) (common.Hash, error) {
	hash := rawdb.ReadCanonicalHash(odr.Database(), number)
	if hash != (common.Hash{}) {
		return hash, nil
	}
	header, err := GetHeaderByNumber(ctx, odr, number)
	if err != nil {
		return common.Hash{}, err
	}
	
	return header.Hash(), nil
}


func GetTd(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (*big.Int, error) {
	td := rawdb.ReadTd(odr.Database(), hash, number)
	if td != nil {
		return td, nil
	}
	_, err := GetHeaderByNumber(ctx, odr, number)
	if err != nil {
		return nil, err
	}
	
	return rawdb.ReadTd(odr.Database(), hash, number), nil
}


func GetBodyRLP(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (rlp.RawValue, error) {
	if data := rawdb.ReadBodyRLP(odr.Database(), hash, number); data != nil {
		return data, nil
	}
	
	header, err := GetHeaderByNumber(ctx, odr, number)
	if err != nil {
		return nil, errNoHeader
	}
	r := &BlockRequest{Hash: hash, Number: number, Header: header}
	if err := odr.Retrieve(ctx, r); err != nil {
		return nil, err
	}
	return r.Rlp, nil
}



func GetBody(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (*types.Body, error) {
	data, err := GetBodyRLP(ctx, odr, hash, number)
	if err != nil {
		return nil, err
	}
	body := new(types.Body)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		return nil, err
	}
	return body, nil
}



func GetBlock(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (*types.Block, error) {
	
	header, err := GetHeaderByNumber(ctx, odr, number)
	if err != nil {
		return nil, errNoHeader
	}
	body, err := GetBody(ctx, odr, hash, number)
	if err != nil {
		return nil, err
	}
	
	return types.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles), nil
}



func GetBlockReceipts(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (types.Receipts, error) {
	
	receipts := rawdb.ReadRawReceipts(odr.Database(), hash, number)
	if receipts == nil {
		header, err := GetHeaderByNumber(ctx, odr, number)
		if err != nil {
			return nil, errNoHeader
		}
		r := &ReceiptsRequest{Hash: hash, Number: number, Header: header}
		if err := odr.Retrieve(ctx, r); err != nil {
			return nil, err
		}
		receipts = r.Receipts
	}
	
	if len(receipts) > 0 && receipts[0].TxHash == (common.Hash{}) {
		block, err := GetBlock(ctx, odr, hash, number)
		if err != nil {
			return nil, err
		}
		genesis := rawdb.ReadCanonicalHash(odr.Database(), 0)
		config := rawdb.ReadChainConfig(odr.Database(), genesis)

		if err := receipts.DeriveFields(config, block.Hash(), block.NumberU64(), block.Transactions()); err != nil {
			return nil, err
		}
		rawdb.WriteReceipts(odr.Database(), hash, number, receipts)
	}
	return receipts, nil
}



func GetBlockLogs(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) ([][]*types.Log, error) {
	
	receipts, err := GetBlockReceipts(ctx, odr, hash, number)
	if err != nil {
		return nil, err
	}
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}




func GetUntrustedBlockLogs(ctx context.Context, odr OdrBackend, header *types.Header) ([][]*types.Log, error) {
	
	hash, number := header.Hash(), header.Number.Uint64()
	receipts := rawdb.ReadRawReceipts(odr.Database(), hash, number)
	if receipts == nil {
		r := &ReceiptsRequest{Hash: hash, Number: number, Header: header, Untrusted: true}
		if err := odr.Retrieve(ctx, r); err != nil {
			return nil, err
		}
		receipts = r.Receipts
		
		
	}
	
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}



func GetBloomBits(ctx context.Context, odr OdrBackend, bit uint, sections []uint64) ([][]byte, error) {
	var (
		reqIndex    []int
		reqSections []uint64
		db          = odr.Database()
		result      = make([][]byte, len(sections))
	)
	blooms, _, sectionHead := odr.BloomTrieIndexer().Sections()
	for i, section := range sections {
		sectionHead := rawdb.ReadCanonicalHash(db, (section+1)*odr.IndexerConfig().BloomSize-1)
		
		
		
		if bloomBits, _ := rawdb.ReadBloomBits(db, bit, section, sectionHead); len(bloomBits) != 0 {
			result[i] = bloomBits
			continue
		}
		
		if section >= blooms {
			return nil, errNoTrustedBloomTrie
		}
		reqSections = append(reqSections, section)
		reqIndex = append(reqIndex, i)
	}
	
	if reqSections == nil {
		return result, nil
	}
	
	r := &BloomRequest{
		BloomTrieRoot:    GetBloomTrieRoot(db, blooms-1, sectionHead),
		BloomTrieNum:     blooms - 1,
		BitIdx:           bit,
		SectionIndexList: reqSections,
		Config:           odr.IndexerConfig(),
	}
	if err := odr.Retrieve(ctx, r); err != nil {
		return nil, err
	}
	for i, idx := range reqIndex {
		result[idx] = r.BloomBits[i]
	}
	return result, nil
}


func GetTransaction(ctx context.Context, odr OdrBackend, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	r := &TxStatusRequest{Hashes: []common.Hash{txHash}}
	if err := odr.Retrieve(ctx, r); err != nil || r.Status[0].Status != core.TxStatusIncluded {
		return nil, common.Hash{}, 0, 0, err
	}
	pos := r.Status[0].Lookup
	
	
	if header, err := GetHeaderByNumber(ctx, odr, pos.BlockIndex); err != nil || header.Hash() != pos.BlockHash {
		return nil, common.Hash{}, 0, 0, err
	}
	body, err := GetBody(ctx, odr, pos.BlockHash, pos.BlockIndex)
	if err != nil || uint64(len(body.Transactions)) <= pos.Index || body.Transactions[pos.Index].Hash() != txHash {
		return nil, common.Hash{}, 0, 0, err
	}
	return body.Transactions[pos.Index], pos.BlockHash, pos.BlockIndex, pos.Index, nil
}
