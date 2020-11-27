















package eth

import (
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rlp"
)


const (
	eth63 = 63
	eth64 = 64
	eth65 = 65
)


const protocolName = "eth"


var ProtocolVersions = []uint{eth65, eth64, eth63}


var protocolLengths = map[uint]uint64{eth65: 17, eth64: 17, eth63: 17}

const protocolMaxMsgSize = 10 * 1024 * 1024 


const (
	StatusMsg          = 0x00
	NewBlockHashesMsg  = 0x01
	TransactionMsg     = 0x02
	GetBlockHeadersMsg = 0x03
	BlockHeadersMsg    = 0x04
	GetBlockBodiesMsg  = 0x05
	BlockBodiesMsg     = 0x06
	NewBlockMsg        = 0x07
	GetNodeDataMsg     = 0x0d
	NodeDataMsg        = 0x0e
	GetReceiptsMsg     = 0x0f
	ReceiptsMsg        = 0x10

	
	
	
	
	NewPooledTransactionHashesMsg = 0x08
	GetPooledTransactionsMsg      = 0x09
	PooledTransactionsMsg         = 0x0a
)

type errCode int

const (
	ErrMsgTooLarge = iota
	ErrDecode
	ErrInvalidMsgCode
	ErrProtocolVersionMismatch
	ErrNetworkIDMismatch
	ErrGenesisMismatch
	ErrForkIDRejected
	ErrNoStatusMsg
	ErrExtraStatusMsg
)

func (e errCode) String() string {
	return errorToString[int(e)]
}


var errorToString = map[int]string{
	ErrMsgTooLarge:             "Message too long",
	ErrDecode:                  "Invalid message",
	ErrInvalidMsgCode:          "Invalid message code",
	ErrProtocolVersionMismatch: "Protocol version mismatch",
	ErrNetworkIDMismatch:       "Network ID mismatch",
	ErrGenesisMismatch:         "Genesis mismatch",
	ErrForkIDRejected:          "Fork ID rejected",
	ErrNoStatusMsg:             "No status message",
	ErrExtraStatusMsg:          "Extra status message",
}

type txPool interface {
	
	
	Has(hash common.Hash) bool

	
	
	Get(hash common.Hash) *types.Transaction

	
	AddRemotes([]*types.Transaction) []error

	
	
	Pending() (map[common.Address]types.Transactions, error)

	
	
	SubscribeNewTxsEvent(chan<- core.NewTxsEvent) event.Subscription
}


type statusData63 struct {
	ProtocolVersion uint32
	NetworkId       uint64
	TD              *big.Int
	CurrentBlock    common.Hash
	GenesisBlock    common.Hash
}


type statusData struct {
	ProtocolVersion uint32
	NetworkID       uint64
	TD              *big.Int
	Head            common.Hash
	Genesis         common.Hash
	ForkID          forkid.ID
}


type newBlockHashesData []struct {
	Hash   common.Hash 
	Number uint64      
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


type newBlockData struct {
	Block *types.Block
	TD    *big.Int
}


func (request *newBlockData) sanityCheck() error {
	if err := request.Block.SanityCheck(); err != nil {
		return err
	}
	
	
	if tdlen := request.TD.BitLen(); tdlen > 100 {
		return fmt.Errorf("too large block TD: bitlen %d", tdlen)
	}
	return nil
}


type blockBody struct {
	Transactions []*types.Transaction 
	Uncles       []*types.Header      
}


type blockBodiesData []*blockBody
