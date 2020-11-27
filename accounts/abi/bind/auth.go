















package bind

import (
	"crypto/ecdsa"
	"errors"
	"io"
	"io/ioutil"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/external"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)



func NewTransactor(keyin io.Reader, passphrase string) (*TransactOpts, error) {
	json, err := ioutil.ReadAll(keyin)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptKey(json, passphrase)
	if err != nil {
		return nil, err
	}
	return NewKeyedTransactor(key.PrivateKey), nil
}



func NewKeyStoreTransactor(keystore *keystore.KeyStore, account accounts.Account) (*TransactOpts, error) {
	return &TransactOpts{
		From: account.Address,
		Signer: func(signer types.Signer, address common.Address, tx *types.Transaction) (*types.Transaction, error) {
			if address != account.Address {
				return nil, errors.New("not authorized to sign this account")
			}
			signature, err := keystore.SignHash(account, signer.Hash(tx).Bytes())
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(signer, signature)
		},
	}, nil
}



func NewKeyedTransactor(key *ecdsa.PrivateKey) *TransactOpts {
	keyAddr := crypto.PubkeyToAddress(key.PublicKey)
	return &TransactOpts{
		From: keyAddr,
		Signer: func(signer types.Signer, address common.Address, tx *types.Transaction) (*types.Transaction, error) {
			if address != keyAddr {
				return nil, errors.New("not authorized to sign this account")
			}
			signature, err := crypto.Sign(signer.Hash(tx).Bytes(), key)
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(signer, signature)
		},
	}
}



func NewClefTransactor(clef *external.ExternalSigner, account accounts.Account) *TransactOpts {
	return &TransactOpts{
		From: account.Address,
		Signer: func(signer types.Signer, address common.Address, transaction *types.Transaction) (*types.Transaction, error) {
			if address != account.Address {
				return nil, errors.New("not authorized to sign this account")
			}
			return clef.SignTx(account, transaction, nil) 
		},
	}
}
