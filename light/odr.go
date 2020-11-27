















package light

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
)



var NoOdr = context.Background()


var ErrNoPeers = errors.New("no suitable peers available")


type OdrBackend interface {
	Database() ethdb.Database
	ChtIndexer() *core.ChainIndexer
	BloomTrieIndexer() *core.ChainIndexer
	BloomIndexer() *core.ChainIndexer
	Retrieve(ctx context.Context, req OdrRequest) error
	IndexerConfig() *IndexerConfig
}


type OdrRequest interface {
	StoreResult(db ethdb.Database)
}


type TrieID struct {
	BlockHash, Root common.Hash
	BlockNumber     uint64
	AccKey          []byte
}



func StateTrieID(header *types.Header) *TrieID {
	return &TrieID{
		BlockHash:   header.Hash(),
		BlockNumber: header.Number.Uint64(),
		AccKey:      nil,
		Root:        header.Root,
	}
}




func StorageTrieID(state *TrieID, addrHash, root common.Hash) *TrieID {
	return &TrieID{
		BlockHash:   state.BlockHash,
		BlockNumber: state.BlockNumber,
		AccKey:      addrHash[:],
		Root:        root,
	}
}


type TrieRequest struct {
	Id    *TrieID
	Key   []byte
	Proof *NodeSet
}


func (req *TrieRequest) StoreResult(db ethdb.Database) {
	req.Proof.Store(db)
}


type CodeRequest struct {
	Id   *TrieID 
	Hash common.Hash
	Data []byte
}


func (req *CodeRequest) StoreResult(db ethdb.Database) {
	rawdb.WriteCode(db, req.Hash, req.Data)
}


type BlockRequest struct {
	Hash   common.Hash
	Number uint64
	Header *types.Header
	Rlp    []byte
}


func (req *BlockRequest) StoreResult(db ethdb.Database) {
	rawdb.WriteBodyRLP(db, req.Hash, req.Number, req.Rlp)
}


type ReceiptsRequest struct {
	Untrusted bool 
	Hash      common.Hash
	Number    uint64
	Header    *types.Header
	Receipts  types.Receipts
}


func (req *ReceiptsRequest) StoreResult(db ethdb.Database) {
	if !req.Untrusted {
		rawdb.WriteReceipts(db, req.Hash, req.Number, req.Receipts)
	}
}


type ChtRequest struct {
	Untrusted        bool   
	PeerId           string 
	Config           *IndexerConfig
	ChtNum, BlockNum uint64
	ChtRoot          common.Hash
	Header           *types.Header
	Td               *big.Int
	Proof            *NodeSet
}


func (req *ChtRequest) StoreResult(db ethdb.Database) {
	hash, num := req.Header.Hash(), req.Header.Number.Uint64()

	if !req.Untrusted {
		rawdb.WriteHeader(db, req.Header)
		rawdb.WriteTd(db, hash, num, req.Td)
		rawdb.WriteCanonicalHash(db, hash, num)
	}
}


type BloomRequest struct {
	OdrRequest
	Config           *IndexerConfig
	BloomTrieNum     uint64
	BitIdx           uint
	SectionIndexList []uint64
	BloomTrieRoot    common.Hash
	BloomBits        [][]byte
	Proofs           *NodeSet
}


func (req *BloomRequest) StoreResult(db ethdb.Database) {
	for i, sectionIdx := range req.SectionIndexList {
		sectionHead := rawdb.ReadCanonicalHash(db, (sectionIdx+1)*req.Config.BloomTrieSize-1)
		
		
		
		
		rawdb.WriteBloomBits(db, req.BitIdx, sectionIdx, sectionHead, req.BloomBits[i])
	}
}


type TxStatus struct {
	Status core.TxStatus
	Lookup *rawdb.LegacyTxLookupEntry `rlp:"nil"`
	Error  string
}


type TxStatusRequest struct {
	Hashes []common.Hash
	Status []TxStatus
}


func (req *TxStatusRequest) StoreResult(db ethdb.Database) {}
