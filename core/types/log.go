















package types

import (
	"io"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
)





type Log struct {
	
	
	Address common.Address `json:"address" gencodec:"required"`
	
	Topics []common.Hash `json:"topics" gencodec:"required"`
	
	Data []byte `json:"data" gencodec:"required"`

	
	
	
	BlockNumber uint64 `json:"blockNumber"`
	
	TxHash common.Hash `json:"transactionHash" gencodec:"required"`
	
	TxIndex uint `json:"transactionIndex"`
	
	BlockHash common.Hash `json:"blockHash"`
	
	Index uint `json:"logIndex"`

	
	
	Removed bool `json:"removed"`
}

type logMarshaling struct {
	Data        hexutil.Bytes
	BlockNumber hexutil.Uint64
	TxIndex     hexutil.Uint
	Index       hexutil.Uint
}

type rlpLog struct {
	Address common.Address
	Topics  []common.Hash
	Data    []byte
}


type rlpStorageLog rlpLog


type legacyRlpStorageLog struct {
	Address     common.Address
	Topics      []common.Hash
	Data        []byte
	BlockNumber uint64
	TxHash      common.Hash
	TxIndex     uint
	BlockHash   common.Hash
	Index       uint
}


func (l *Log) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, rlpLog{Address: l.Address, Topics: l.Topics, Data: l.Data})
}


func (l *Log) DecodeRLP(s *rlp.Stream) error {
	var dec rlpLog
	err := s.Decode(&dec)
	if err == nil {
		l.Address, l.Topics, l.Data = dec.Address, dec.Topics, dec.Data
	}
	return err
}



type LogForStorage Log


func (l *LogForStorage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, rlpStorageLog{
		Address: l.Address,
		Topics:  l.Topics,
		Data:    l.Data,
	})
}




func (l *LogForStorage) DecodeRLP(s *rlp.Stream) error {
	blob, err := s.Raw()
	if err != nil {
		return err
	}
	var dec rlpStorageLog
	err = rlp.DecodeBytes(blob, &dec)
	if err == nil {
		*l = LogForStorage{
			Address: dec.Address,
			Topics:  dec.Topics,
			Data:    dec.Data,
		}
	} else {
		
		var dec legacyRlpStorageLog
		err = rlp.DecodeBytes(blob, &dec)
		if err == nil {
			*l = LogForStorage{
				Address: dec.Address,
				Topics:  dec.Topics,
				Data:    dec.Data,
			}
		}
	}
	return err
}
