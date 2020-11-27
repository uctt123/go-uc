















package types

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)



var (
	receiptStatusFailedRLP     = []byte{}
	receiptStatusSuccessfulRLP = []byte{0x01}
)

const (
	
	ReceiptStatusFailed = uint64(0)

	
	ReceiptStatusSuccessful = uint64(1)
)


type Receipt struct {
	
	PostState         []byte `json:"root"`
	Status            uint64 `json:"status"`
	CumulativeGasUsed uint64 `json:"cumulativeGasUsed" gencodec:"required"`
	Bloom             Bloom  `json:"logsBloom"         gencodec:"required"`
	Logs              []*Log `json:"logs"              gencodec:"required"`

	
	
	TxHash          common.Hash    `json:"transactionHash" gencodec:"required"`
	ContractAddress common.Address `json:"contractAddress"`
	GasUsed         uint64         `json:"gasUsed" gencodec:"required"`

	
	
	BlockHash        common.Hash `json:"blockHash,omitempty"`
	BlockNumber      *big.Int    `json:"blockNumber,omitempty"`
	TransactionIndex uint        `json:"transactionIndex"`
}

type receiptMarshaling struct {
	PostState         hexutil.Bytes
	Status            hexutil.Uint64
	CumulativeGasUsed hexutil.Uint64
	GasUsed           hexutil.Uint64
	BlockNumber       *hexutil.Big
	TransactionIndex  hexutil.Uint
}


type receiptRLP struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	Bloom             Bloom
	Logs              []*Log
}


type storedReceiptRLP struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	Logs              []*LogForStorage
}


type v4StoredReceiptRLP struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	TxHash            common.Hash
	ContractAddress   common.Address
	Logs              []*LogForStorage
	GasUsed           uint64
}


type v3StoredReceiptRLP struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	Bloom             Bloom
	TxHash            common.Hash
	ContractAddress   common.Address
	Logs              []*LogForStorage
	GasUsed           uint64
}


func NewReceipt(root []byte, failed bool, cumulativeGasUsed uint64) *Receipt {
	r := &Receipt{PostState: common.CopyBytes(root), CumulativeGasUsed: cumulativeGasUsed}
	if failed {
		r.Status = ReceiptStatusFailed
	} else {
		r.Status = ReceiptStatusSuccessful
	}
	return r
}



func (r *Receipt) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &receiptRLP{r.statusEncoding(), r.CumulativeGasUsed, r.Bloom, r.Logs})
}



func (r *Receipt) DecodeRLP(s *rlp.Stream) error {
	var dec receiptRLP
	if err := s.Decode(&dec); err != nil {
		return err
	}
	if err := r.setStatus(dec.PostStateOrStatus); err != nil {
		return err
	}
	r.CumulativeGasUsed, r.Bloom, r.Logs = dec.CumulativeGasUsed, dec.Bloom, dec.Logs
	return nil
}

func (r *Receipt) setStatus(postStateOrStatus []byte) error {
	switch {
	case bytes.Equal(postStateOrStatus, receiptStatusSuccessfulRLP):
		r.Status = ReceiptStatusSuccessful
	case bytes.Equal(postStateOrStatus, receiptStatusFailedRLP):
		r.Status = ReceiptStatusFailed
	case len(postStateOrStatus) == len(common.Hash{}):
		r.PostState = postStateOrStatus
	default:
		return fmt.Errorf("invalid receipt status %x", postStateOrStatus)
	}
	return nil
}

func (r *Receipt) statusEncoding() []byte {
	if len(r.PostState) == 0 {
		if r.Status == ReceiptStatusFailed {
			return receiptStatusFailedRLP
		}
		return receiptStatusSuccessfulRLP
	}
	return r.PostState
}



func (r *Receipt) Size() common.StorageSize {
	size := common.StorageSize(unsafe.Sizeof(*r)) + common.StorageSize(len(r.PostState))

	size += common.StorageSize(len(r.Logs)) * common.StorageSize(unsafe.Sizeof(Log{}))
	for _, log := range r.Logs {
		size += common.StorageSize(len(log.Topics)*common.HashLength + len(log.Data))
	}
	return size
}



type ReceiptForStorage Receipt



func (r *ReceiptForStorage) EncodeRLP(w io.Writer) error {
	enc := &storedReceiptRLP{
		PostStateOrStatus: (*Receipt)(r).statusEncoding(),
		CumulativeGasUsed: r.CumulativeGasUsed,
		Logs:              make([]*LogForStorage, len(r.Logs)),
	}
	for i, log := range r.Logs {
		enc.Logs[i] = (*LogForStorage)(log)
	}
	return rlp.Encode(w, enc)
}



func (r *ReceiptForStorage) DecodeRLP(s *rlp.Stream) error {
	
	blob, err := s.Raw()
	if err != nil {
		return err
	}
	
	
	
	if err := decodeStoredReceiptRLP(r, blob); err == nil {
		return nil
	}
	if err := decodeV3StoredReceiptRLP(r, blob); err == nil {
		return nil
	}
	return decodeV4StoredReceiptRLP(r, blob)
}

func decodeStoredReceiptRLP(r *ReceiptForStorage, blob []byte) error {
	var stored storedReceiptRLP
	if err := rlp.DecodeBytes(blob, &stored); err != nil {
		return err
	}
	if err := (*Receipt)(r).setStatus(stored.PostStateOrStatus); err != nil {
		return err
	}
	r.CumulativeGasUsed = stored.CumulativeGasUsed
	r.Logs = make([]*Log, len(stored.Logs))
	for i, log := range stored.Logs {
		r.Logs[i] = (*Log)(log)
	}
	r.Bloom = CreateBloom(Receipts{(*Receipt)(r)})

	return nil
}

func decodeV4StoredReceiptRLP(r *ReceiptForStorage, blob []byte) error {
	var stored v4StoredReceiptRLP
	if err := rlp.DecodeBytes(blob, &stored); err != nil {
		return err
	}
	if err := (*Receipt)(r).setStatus(stored.PostStateOrStatus); err != nil {
		return err
	}
	r.CumulativeGasUsed = stored.CumulativeGasUsed
	r.TxHash = stored.TxHash
	r.ContractAddress = stored.ContractAddress
	r.GasUsed = stored.GasUsed
	r.Logs = make([]*Log, len(stored.Logs))
	for i, log := range stored.Logs {
		r.Logs[i] = (*Log)(log)
	}
	r.Bloom = CreateBloom(Receipts{(*Receipt)(r)})

	return nil
}

func decodeV3StoredReceiptRLP(r *ReceiptForStorage, blob []byte) error {
	var stored v3StoredReceiptRLP
	if err := rlp.DecodeBytes(blob, &stored); err != nil {
		return err
	}
	if err := (*Receipt)(r).setStatus(stored.PostStateOrStatus); err != nil {
		return err
	}
	r.CumulativeGasUsed = stored.CumulativeGasUsed
	r.Bloom = stored.Bloom
	r.TxHash = stored.TxHash
	r.ContractAddress = stored.ContractAddress
	r.GasUsed = stored.GasUsed
	r.Logs = make([]*Log, len(stored.Logs))
	for i, log := range stored.Logs {
		r.Logs[i] = (*Log)(log)
	}
	return nil
}


type Receipts []*Receipt


func (r Receipts) Len() int { return len(r) }


func (r Receipts) GetRlp(i int) []byte {
	bytes, err := rlp.EncodeToBytes(r[i])
	if err != nil {
		panic(err)
	}
	return bytes
}



func (r Receipts) DeriveFields(config *params.ChainConfig, hash common.Hash, number uint64, txs Transactions) error {
	signer := MakeSigner(config, new(big.Int).SetUint64(number))

	logIndex := uint(0)
	if len(txs) != len(r) {
		return errors.New("transaction and receipt count mismatch")
	}
	for i := 0; i < len(r); i++ {
		
		r[i].TxHash = txs[i].Hash()

		
		r[i].BlockHash = hash
		r[i].BlockNumber = new(big.Int).SetUint64(number)
		r[i].TransactionIndex = uint(i)

		
		if txs[i].To() == nil {
			
			from, _ := Sender(signer, txs[i])
			r[i].ContractAddress = crypto.CreateAddress(from, txs[i].Nonce())
		}
		
		if i == 0 {
			r[i].GasUsed = r[i].CumulativeGasUsed
		} else {
			r[i].GasUsed = r[i].CumulativeGasUsed - r[i-1].CumulativeGasUsed
		}
		
		for j := 0; j < len(r[i].Logs); j++ {
			r[i].Logs[j].BlockNumber = number
			r[i].Logs[j].BlockHash = hash
			r[i].Logs[j].TxHash = r[i].TxHash
			r[i].Logs[j].TxIndex = uint(i)
			r[i].Logs[j].Index = logIndex
			logIndex++
		}
	}
	return nil
}
