















package les

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	lpc "github.com/ethereum/go-ethereum/les/lespay/client"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)


const (
	lpv2 = 2
	lpv3 = 3
)


var (
	ClientProtocolVersions    = []uint{lpv2, lpv3}
	ServerProtocolVersions    = []uint{lpv2, lpv3}
	AdvertiseProtocolVersions = []uint{lpv2} 
)


var ProtocolLengths = map[uint]uint64{lpv2: 22, lpv3: 24}

const (
	NetworkId          = 1
	ProtocolMaxMsgSize = 10 * 1024 * 1024 
)


const (
	
	StatusMsg          = 0x00
	AnnounceMsg        = 0x01
	GetBlockHeadersMsg = 0x02
	BlockHeadersMsg    = 0x03
	GetBlockBodiesMsg  = 0x04
	BlockBodiesMsg     = 0x05
	GetReceiptsMsg     = 0x06
	ReceiptsMsg        = 0x07
	GetCodeMsg         = 0x0a
	CodeMsg            = 0x0b
	
	GetProofsV2Msg         = 0x0f
	ProofsV2Msg            = 0x10
	GetHelperTrieProofsMsg = 0x11
	HelperTrieProofsMsg    = 0x12
	SendTxV2Msg            = 0x13
	GetTxStatusMsg         = 0x14
	TxStatusMsg            = 0x15
	
	StopMsg   = 0x16
	ResumeMsg = 0x17
)

type requestInfo struct {
	name                          string
	maxCount                      uint64
	refBasketFirst, refBasketRest float64
}




type reqMapping struct {
	first, rest int
}

var (
	
	
	
	requests = map[uint64]requestInfo{
		GetBlockHeadersMsg:     {"GetBlockHeaders", MaxHeaderFetch, 10, 1000},
		GetBlockBodiesMsg:      {"GetBlockBodies", MaxBodyFetch, 1, 0},
		GetReceiptsMsg:         {"GetReceipts", MaxReceiptFetch, 1, 0},
		GetCodeMsg:             {"GetCode", MaxCodeFetch, 1, 0},
		GetProofsV2Msg:         {"GetProofsV2", MaxProofsFetch, 10, 0},
		GetHelperTrieProofsMsg: {"GetHelperTrieProofs", MaxHelperTrieProofsFetch, 10, 100},
		SendTxV2Msg:            {"SendTxV2", MaxTxSend, 1, 0},
		GetTxStatusMsg:         {"GetTxStatus", MaxTxStatus, 10, 0},
	}
	requestList    []lpc.RequestInfo
	requestMapping map[uint32]reqMapping
)



func init() {
	requestMapping = make(map[uint32]reqMapping)
	for code, req := range requests {
		cost := reqAvgTimeCost[code]
		rm := reqMapping{len(requestList), -1}
		requestList = append(requestList, lpc.RequestInfo{
			Name:       req.name + ".first",
			InitAmount: req.refBasketFirst,
			InitValue:  float64(cost.baseCost + cost.reqCost),
		})
		if req.refBasketRest != 0 {
			rm.rest = len(requestList)
			requestList = append(requestList, lpc.RequestInfo{
				Name:       req.name + ".rest",
				InitAmount: req.refBasketRest,
				InitValue:  float64(cost.reqCost),
			})
		}
		requestMapping[uint32(code)] = rm
	}
}

type errCode int

const (
	ErrMsgTooLarge = iota
	ErrDecode
	ErrInvalidMsgCode
	ErrProtocolVersionMismatch
	ErrNetworkIdMismatch
	ErrGenesisBlockMismatch
	ErrNoStatusMsg
	ErrExtraStatusMsg
	ErrSuspendedPeer
	ErrUselessPeer
	ErrRequestRejected
	ErrUnexpectedResponse
	ErrInvalidResponse
	ErrTooManyTimeouts
	ErrMissingKey
)

func (e errCode) String() string {
	return errorToString[int(e)]
}


var errorToString = map[int]string{
	ErrMsgTooLarge:             "Message too long",
	ErrDecode:                  "Invalid message",
	ErrInvalidMsgCode:          "Invalid message code",
	ErrProtocolVersionMismatch: "Protocol version mismatch",
	ErrNetworkIdMismatch:       "NetworkId mismatch",
	ErrGenesisBlockMismatch:    "Genesis block mismatch",
	ErrNoStatusMsg:             "No status message",
	ErrExtraStatusMsg:          "Extra status message",
	ErrSuspendedPeer:           "Suspended peer",
	ErrRequestRejected:         "Request rejected",
	ErrUnexpectedResponse:      "Unexpected response",
	ErrInvalidResponse:         "Invalid response",
	ErrTooManyTimeouts:         "Too many request timeouts",
	ErrMissingKey:              "Key missing from list",
}

type announceBlock struct {
	Hash   common.Hash 
	Number uint64      
	Td     *big.Int    
}


type announceData struct {
	Hash       common.Hash 
	Number     uint64      
	Td         *big.Int    
	ReorgDepth uint64
	Update     keyValueList
}


func (a *announceData) sanityCheck() error {
	if tdlen := a.Td.BitLen(); tdlen > 100 {
		return fmt.Errorf("too large block TD: bitlen %d", tdlen)
	}
	return nil
}


func (a *announceData) sign(privKey *ecdsa.PrivateKey) {
	rlp, _ := rlp.EncodeToBytes(announceBlock{a.Hash, a.Number, a.Td})
	sig, _ := crypto.Sign(crypto.Keccak256(rlp), privKey)
	a.Update = a.Update.add("sign", sig)
}


func (a *announceData) checkSignature(id enode.ID, update keyValueMap) error {
	var sig []byte
	if err := update.get("sign", &sig); err != nil {
		return err
	}
	rlp, _ := rlp.EncodeToBytes(announceBlock{a.Hash, a.Number, a.Td})
	recPubkey, err := crypto.SigToPub(crypto.Keccak256(rlp), sig)
	if err != nil {
		return err
	}
	if id == enode.PubkeyToIDV4(recPubkey) {
		return nil
	}
	return errors.New("wrong signature")
}

type blockInfo struct {
	Hash   common.Hash 
	Number uint64      
	Td     *big.Int    
}


type getBlockHeadersData struct {
	Origin  hashOrNumber 
	Amount  uint64       
	Skip    uint64       
	Reverse bool         
}


type hashOrNumber struct {
	Hash   common.Hash 
	Number uint64      
}



func (hn *hashOrNumber) EncodeRLP(w io.Writer) error {
	if hn.Hash == (common.Hash{}) {
		return rlp.Encode(w, hn.Number)
	}
	if hn.Number != 0 {
		return fmt.Errorf("both origin hash (%x) and number (%d) provided", hn.Hash, hn.Number)
	}
	return rlp.Encode(w, hn.Hash)
}



func (hn *hashOrNumber) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	origin, err := s.Raw()
	if err == nil {
		switch {
		case size == 32:
			err = rlp.DecodeBytes(origin, &hn.Hash)
		case size <= 8:
			err = rlp.DecodeBytes(origin, &hn.Number)
		default:
			err = fmt.Errorf("invalid input size %d for origin", size)
		}
	}
	return err
}


type CodeData []struct {
	Value []byte
}
