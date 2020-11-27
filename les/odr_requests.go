















package les

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

var (
	errInvalidMessageType  = errors.New("invalid message type")
	errInvalidEntryCount   = errors.New("invalid number of response entries")
	errHeaderUnavailable   = errors.New("header unavailable")
	errTxHashMismatch      = errors.New("transaction hash mismatch")
	errUncleHashMismatch   = errors.New("uncle hash mismatch")
	errReceiptHashMismatch = errors.New("receipt hash mismatch")
	errDataHashMismatch    = errors.New("data hash mismatch")
	errCHTHashMismatch     = errors.New("cht hash mismatch")
	errCHTNumberMismatch   = errors.New("cht number mismatch")
	errUselessNodes        = errors.New("useless nodes in merkle proof nodeset")
)

type LesOdrRequest interface {
	GetCost(*serverPeer) uint64
	CanSend(*serverPeer) bool
	Request(uint64, *serverPeer) error
	Validate(ethdb.Database, *Msg) error
}

func LesRequest(req light.OdrRequest) LesOdrRequest {
	switch r := req.(type) {
	case *light.BlockRequest:
		return (*BlockRequest)(r)
	case *light.ReceiptsRequest:
		return (*ReceiptsRequest)(r)
	case *light.TrieRequest:
		return (*TrieRequest)(r)
	case *light.CodeRequest:
		return (*CodeRequest)(r)
	case *light.ChtRequest:
		return (*ChtRequest)(r)
	case *light.BloomRequest:
		return (*BloomRequest)(r)
	case *light.TxStatusRequest:
		return (*TxStatusRequest)(r)
	default:
		return nil
	}
}


type BlockRequest light.BlockRequest



func (r *BlockRequest) GetCost(peer *serverPeer) uint64 {
	return peer.getRequestCost(GetBlockBodiesMsg, 1)
}


func (r *BlockRequest) CanSend(peer *serverPeer) bool {
	return peer.HasBlock(r.Hash, r.Number, false)
}


func (r *BlockRequest) Request(reqID uint64, peer *serverPeer) error {
	peer.Log().Debug("Requesting block body", "hash", r.Hash)
	return peer.requestBodies(reqID, []common.Hash{r.Hash})
}




func (r *BlockRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating block body", "hash", r.Hash)

	
	if msg.MsgType != MsgBlockBodies {
		return errInvalidMessageType
	}
	bodies := msg.Obj.([]*types.Body)
	if len(bodies) != 1 {
		return errInvalidEntryCount
	}
	body := bodies[0]

	
	if r.Header == nil {
		r.Header = rawdb.ReadHeader(db, r.Hash, r.Number)
	}
	if r.Header == nil {
		return errHeaderUnavailable
	}
	if r.Header.TxHash != types.DeriveSha(types.Transactions(body.Transactions), new(trie.Trie)) {
		return errTxHashMismatch
	}
	if r.Header.UncleHash != types.CalcUncleHash(body.Uncles) {
		return errUncleHashMismatch
	}
	
	data, err := rlp.EncodeToBytes(body)
	if err != nil {
		return err
	}
	r.Rlp = data
	return nil
}


type ReceiptsRequest light.ReceiptsRequest



func (r *ReceiptsRequest) GetCost(peer *serverPeer) uint64 {
	return peer.getRequestCost(GetReceiptsMsg, 1)
}


func (r *ReceiptsRequest) CanSend(peer *serverPeer) bool {
	return peer.HasBlock(r.Hash, r.Number, false)
}


func (r *ReceiptsRequest) Request(reqID uint64, peer *serverPeer) error {
	peer.Log().Debug("Requesting block receipts", "hash", r.Hash)
	return peer.requestReceipts(reqID, []common.Hash{r.Hash})
}




func (r *ReceiptsRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating block receipts", "hash", r.Hash)

	
	if msg.MsgType != MsgReceipts {
		return errInvalidMessageType
	}
	receipts := msg.Obj.([]types.Receipts)
	if len(receipts) != 1 {
		return errInvalidEntryCount
	}
	receipt := receipts[0]

	
	if r.Header == nil {
		r.Header = rawdb.ReadHeader(db, r.Hash, r.Number)
	}
	if r.Header == nil {
		return errHeaderUnavailable
	}
	if r.Header.ReceiptHash != types.DeriveSha(receipt, new(trie.Trie)) {
		return errReceiptHashMismatch
	}
	
	r.Receipts = receipt
	return nil
}

type ProofReq struct {
	BHash       common.Hash
	AccKey, Key []byte
	FromLevel   uint
}


type TrieRequest light.TrieRequest



func (r *TrieRequest) GetCost(peer *serverPeer) uint64 {
	return peer.getRequestCost(GetProofsV2Msg, 1)
}


func (r *TrieRequest) CanSend(peer *serverPeer) bool {
	return peer.HasBlock(r.Id.BlockHash, r.Id.BlockNumber, true)
}


func (r *TrieRequest) Request(reqID uint64, peer *serverPeer) error {
	peer.Log().Debug("Requesting trie proof", "root", r.Id.Root, "key", r.Key)
	req := ProofReq{
		BHash:  r.Id.BlockHash,
		AccKey: r.Id.AccKey,
		Key:    r.Key,
	}
	return peer.requestProofs(reqID, []ProofReq{req})
}




func (r *TrieRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating trie proof", "root", r.Id.Root, "key", r.Key)

	if msg.MsgType != MsgProofsV2 {
		return errInvalidMessageType
	}
	proofs := msg.Obj.(light.NodeList)
	
	nodeSet := proofs.NodeSet()
	reads := &readTraceDB{db: nodeSet}
	if _, err := trie.VerifyProof(r.Id.Root, r.Key, reads); err != nil {
		return fmt.Errorf("merkle proof verification failed: %v", err)
	}
	
	if len(reads.reads) != nodeSet.KeyCount() {
		return errUselessNodes
	}
	r.Proof = nodeSet
	return nil
}

type CodeReq struct {
	BHash  common.Hash
	AccKey []byte
}


type CodeRequest light.CodeRequest



func (r *CodeRequest) GetCost(peer *serverPeer) uint64 {
	return peer.getRequestCost(GetCodeMsg, 1)
}


func (r *CodeRequest) CanSend(peer *serverPeer) bool {
	return peer.HasBlock(r.Id.BlockHash, r.Id.BlockNumber, true)
}


func (r *CodeRequest) Request(reqID uint64, peer *serverPeer) error {
	peer.Log().Debug("Requesting code data", "hash", r.Hash)
	req := CodeReq{
		BHash:  r.Id.BlockHash,
		AccKey: r.Id.AccKey,
	}
	return peer.requestCode(reqID, []CodeReq{req})
}




func (r *CodeRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating code data", "hash", r.Hash)

	
	if msg.MsgType != MsgCode {
		return errInvalidMessageType
	}
	reply := msg.Obj.([][]byte)
	if len(reply) != 1 {
		return errInvalidEntryCount
	}
	data := reply[0]

	
	if hash := crypto.Keccak256Hash(data); r.Hash != hash {
		return errDataHashMismatch
	}
	r.Data = data
	return nil
}

const (
	
	htCanonical = iota 
	htBloomBits        

	
	auxRoot = 1
	
	auxHeader = 2
)

type HelperTrieReq struct {
	Type              uint
	TrieIdx           uint64
	Key               []byte
	FromLevel, AuxReq uint
}

type HelperTrieResps struct { 
	Proofs  light.NodeList
	AuxData [][]byte
}


type ChtRequest light.ChtRequest



func (r *ChtRequest) GetCost(peer *serverPeer) uint64 {
	return peer.getRequestCost(GetHelperTrieProofsMsg, 1)
}


func (r *ChtRequest) CanSend(peer *serverPeer) bool {
	peer.lock.RLock()
	defer peer.lock.RUnlock()

	if r.Untrusted {
		return peer.headInfo.Number >= r.BlockNum && peer.id == r.PeerId
	} else {
		return peer.headInfo.Number >= r.Config.ChtConfirms && r.ChtNum <= (peer.headInfo.Number-r.Config.ChtConfirms)/r.Config.ChtSize
	}
}


func (r *ChtRequest) Request(reqID uint64, peer *serverPeer) error {
	peer.Log().Debug("Requesting CHT", "cht", r.ChtNum, "block", r.BlockNum)
	var encNum [8]byte
	binary.BigEndian.PutUint64(encNum[:], r.BlockNum)
	req := HelperTrieReq{
		Type:    htCanonical,
		TrieIdx: r.ChtNum,
		Key:     encNum[:],
		AuxReq:  auxHeader,
	}
	return peer.requestHelperTrieProofs(reqID, []HelperTrieReq{req})
}




func (r *ChtRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating CHT", "cht", r.ChtNum, "block", r.BlockNum)

	if msg.MsgType != MsgHelperTrieProofs {
		return errInvalidMessageType
	}
	resp := msg.Obj.(HelperTrieResps)
	if len(resp.AuxData) != 1 {
		return errInvalidEntryCount
	}
	nodeSet := resp.Proofs.NodeSet()
	headerEnc := resp.AuxData[0]
	if len(headerEnc) == 0 {
		return errHeaderUnavailable
	}
	header := new(types.Header)
	if err := rlp.DecodeBytes(headerEnc, header); err != nil {
		return errHeaderUnavailable
	}

	
	
	
	var node light.ChtNode
	if !r.Untrusted {
		var encNumber [8]byte
		binary.BigEndian.PutUint64(encNumber[:], r.BlockNum)

		reads := &readTraceDB{db: nodeSet}
		value, err := trie.VerifyProof(r.ChtRoot, encNumber[:], reads)
		if err != nil {
			return fmt.Errorf("merkle proof verification failed: %v", err)
		}
		if len(reads.reads) != nodeSet.KeyCount() {
			return errUselessNodes
		}

		if err := rlp.DecodeBytes(value, &node); err != nil {
			return err
		}
		if node.Hash != header.Hash() {
			return errCHTHashMismatch
		}
		if r.BlockNum != header.Number.Uint64() {
			return errCHTNumberMismatch
		}
	}
	
	r.Header = header
	r.Proof = nodeSet
	r.Td = node.Td 

	return nil
}

type BloomReq struct {
	BloomTrieNum, BitIdx, SectionIndex, FromLevel uint64
}


type BloomRequest light.BloomRequest



func (r *BloomRequest) GetCost(peer *serverPeer) uint64 {
	return peer.getRequestCost(GetHelperTrieProofsMsg, len(r.SectionIndexList))
}


func (r *BloomRequest) CanSend(peer *serverPeer) bool {
	peer.lock.RLock()
	defer peer.lock.RUnlock()

	if peer.version < lpv2 {
		return false
	}
	return peer.headInfo.Number >= r.Config.BloomTrieConfirms && r.BloomTrieNum <= (peer.headInfo.Number-r.Config.BloomTrieConfirms)/r.Config.BloomTrieSize
}


func (r *BloomRequest) Request(reqID uint64, peer *serverPeer) error {
	peer.Log().Debug("Requesting BloomBits", "bloomTrie", r.BloomTrieNum, "bitIdx", r.BitIdx, "sections", r.SectionIndexList)
	reqs := make([]HelperTrieReq, len(r.SectionIndexList))

	var encNumber [10]byte
	binary.BigEndian.PutUint16(encNumber[:2], uint16(r.BitIdx))

	for i, sectionIdx := range r.SectionIndexList {
		binary.BigEndian.PutUint64(encNumber[2:], sectionIdx)
		reqs[i] = HelperTrieReq{
			Type:    htBloomBits,
			TrieIdx: r.BloomTrieNum,
			Key:     common.CopyBytes(encNumber[:]),
		}
	}
	return peer.requestHelperTrieProofs(reqID, reqs)
}




func (r *BloomRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating BloomBits", "bloomTrie", r.BloomTrieNum, "bitIdx", r.BitIdx, "sections", r.SectionIndexList)

	
	if msg.MsgType != MsgHelperTrieProofs {
		return errInvalidMessageType
	}
	resps := msg.Obj.(HelperTrieResps)
	proofs := resps.Proofs
	nodeSet := proofs.NodeSet()
	reads := &readTraceDB{db: nodeSet}

	r.BloomBits = make([][]byte, len(r.SectionIndexList))

	
	var encNumber [10]byte
	binary.BigEndian.PutUint16(encNumber[:2], uint16(r.BitIdx))

	for i, idx := range r.SectionIndexList {
		binary.BigEndian.PutUint64(encNumber[2:], idx)
		value, err := trie.VerifyProof(r.BloomTrieRoot, encNumber[:], reads)
		if err != nil {
			return err
		}
		r.BloomBits[i] = value
	}

	if len(reads.reads) != nodeSet.KeyCount() {
		return errUselessNodes
	}
	r.Proofs = nodeSet
	return nil
}


type TxStatusRequest light.TxStatusRequest



func (r *TxStatusRequest) GetCost(peer *serverPeer) uint64 {
	return peer.getRequestCost(GetTxStatusMsg, len(r.Hashes))
}


func (r *TxStatusRequest) CanSend(peer *serverPeer) bool {
	return peer.version >= lpv2
}


func (r *TxStatusRequest) Request(reqID uint64, peer *serverPeer) error {
	peer.Log().Debug("Requesting transaction status", "count", len(r.Hashes))
	return peer.requestTxStatus(reqID, r.Hashes)
}




func (r *TxStatusRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating transaction status", "count", len(r.Hashes))

	
	if msg.MsgType != MsgTxStatus {
		return errInvalidMessageType
	}
	status := msg.Obj.([]light.TxStatus)
	if len(status) != len(r.Hashes) {
		return errInvalidEntryCount
	}
	r.Status = status
	return nil
}



type readTraceDB struct {
	db    ethdb.KeyValueReader
	reads map[string]struct{}
}


func (db *readTraceDB) Get(k []byte) ([]byte, error) {
	if db.reads == nil {
		db.reads = make(map[string]struct{})
	}
	db.reads[string(k)] = struct{}{}
	return db.db.Get(k)
}


func (db *readTraceDB) Has(key []byte) (bool, error) {
	_, err := db.Get(key)
	return err == nil, nil
}
