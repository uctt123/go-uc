















package tests

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)


type TransactionTest struct {
	RLP            hexutil.Bytes `json:"rlp"`
	Byzantium      ttFork
	Constantinople ttFork
	Istanbul       ttFork
	EIP150         ttFork
	EIP158         ttFork
	Frontier       ttFork
	Homestead      ttFork
}

type ttFork struct {
	Sender common.UnprefixedAddress `json:"sender"`
	Hash   common.UnprefixedHash    `json:"hash"`
}

func (tt *TransactionTest) Run(config *params.ChainConfig) error {
	validateTx := func(rlpData hexutil.Bytes, signer types.Signer, isHomestead bool, isIstanbul bool) (*common.Address, *common.Hash, error) {
		tx := new(types.Transaction)
		if err := rlp.DecodeBytes(rlpData, tx); err != nil {
			return nil, nil, err
		}
		sender, err := types.Sender(signer, tx)
		if err != nil {
			return nil, nil, err
		}
		
		requiredGas, err := core.IntrinsicGas(tx.Data(), tx.To() == nil, isHomestead, isIstanbul)
		if err != nil {
			return nil, nil, err
		}
		if requiredGas > tx.Gas() {
			return nil, nil, fmt.Errorf("insufficient gas ( %d < %d )", tx.Gas(), requiredGas)
		}
		h := tx.Hash()
		return &sender, &h, nil
	}

	for _, testcase := range []struct {
		name        string
		signer      types.Signer
		fork        ttFork
		isHomestead bool
		isIstanbul  bool
	}{
		{"Frontier", types.FrontierSigner{}, tt.Frontier, false, false},
		{"Homestead", types.HomesteadSigner{}, tt.Homestead, true, false},
		{"EIP150", types.HomesteadSigner{}, tt.EIP150, true, false},
		{"EIP158", types.NewEIP155Signer(config.ChainID), tt.EIP158, true, false},
		{"Byzantium", types.NewEIP155Signer(config.ChainID), tt.Byzantium, true, false},
		{"Constantinople", types.NewEIP155Signer(config.ChainID), tt.Constantinople, true, false},
		{"Istanbul", types.NewEIP155Signer(config.ChainID), tt.Istanbul, true, true},
	} {
		sender, txhash, err := validateTx(tt.RLP, testcase.signer, testcase.isHomestead, testcase.isIstanbul)

		if testcase.fork.Sender == (common.UnprefixedAddress{}) {
			if err == nil {
				return fmt.Errorf("expected error, got none (address %v)[%v]", sender.String(), testcase.name)
			}
			continue
		}
		
		if err != nil {
			return fmt.Errorf("got error, expected none: %v", err)
		}
		if sender == nil {
			return fmt.Errorf("sender was nil, should be %x", common.Address(testcase.fork.Sender))
		}
		if *sender != common.Address(testcase.fork.Sender) {
			return fmt.Errorf("sender mismatch: got %x, want %x", sender, testcase.fork.Sender)
		}
		if txhash == nil {
			return fmt.Errorf("txhash was nil, should be %x", common.Hash(testcase.fork.Hash))
		}
		if *txhash != common.Hash(testcase.fork.Hash) {
			return fmt.Errorf("hash mismatch: got %x, want %x", *txhash, testcase.fork.Hash)
		}
	}
	return nil
}
