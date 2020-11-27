















package snapshot

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)





type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     []byte
	CodeHash []byte
}


func SlimAccount(nonce uint64, balance *big.Int, root common.Hash, codehash []byte) Account {
	slim := Account{
		Nonce:   nonce,
		Balance: balance,
	}
	if root != emptyRoot {
		slim.Root = root[:]
	}
	if !bytes.Equal(codehash, emptyCode[:]) {
		slim.CodeHash = codehash
	}
	return slim
}



func SlimAccountRLP(nonce uint64, balance *big.Int, root common.Hash, codehash []byte) []byte {
	data, err := rlp.EncodeToBytes(SlimAccount(nonce, balance, root, codehash))
	if err != nil {
		panic(err)
	}
	return data
}



func FullAccount(data []byte) (Account, error) {
	var account Account
	if err := rlp.DecodeBytes(data, &account); err != nil {
		return Account{}, err
	}
	if len(account.Root) == 0 {
		account.Root = emptyRoot[:]
	}
	if len(account.CodeHash) == 0 {
		account.CodeHash = emptyCode[:]
	}
	return account, nil
}


func FullAccountRLP(data []byte) ([]byte, error) {
	account, err := FullAccount(data)
	if err != nil {
		return nil, err
	}
	return rlp.EncodeToBytes(account)
}
