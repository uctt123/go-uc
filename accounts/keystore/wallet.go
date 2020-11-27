















package keystore

import (
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)



type keystoreWallet struct {
	account  accounts.Account 
	keystore *KeyStore        
}


func (w *keystoreWallet) URL() accounts.URL {
	return w.account.URL
}



func (w *keystoreWallet) Status() (string, error) {
	w.keystore.mu.RLock()
	defer w.keystore.mu.RUnlock()

	if _, ok := w.keystore.unlocked[w.account.Address]; ok {
		return "Unlocked", nil
	}
	return "Locked", nil
}



func (w *keystoreWallet) Open(passphrase string) error { return nil }



func (w *keystoreWallet) Close() error { return nil }



func (w *keystoreWallet) Accounts() []accounts.Account {
	return []accounts.Account{w.account}
}



func (w *keystoreWallet) Contains(account accounts.Account) bool {
	return account.Address == w.account.Address && (account.URL == (accounts.URL{}) || account.URL == w.account.URL)
}



func (w *keystoreWallet) Derive(path accounts.DerivationPath, pin bool) (accounts.Account, error) {
	return accounts.Account{}, accounts.ErrNotSupported
}



func (w *keystoreWallet) SelfDerive(bases []accounts.DerivationPath, chain ethereum.ChainStateReader) {
}





func (w *keystoreWallet) signHash(account accounts.Account, hash []byte) ([]byte, error) {
	
	if !w.Contains(account) {
		return nil, accounts.ErrUnknownAccount
	}
	
	return w.keystore.SignHash(account, hash)
}


func (w *keystoreWallet) SignData(account accounts.Account, mimeType string, data []byte) ([]byte, error) {
	return w.signHash(account, crypto.Keccak256(data))
}


func (w *keystoreWallet) SignDataWithPassphrase(account accounts.Account, passphrase, mimeType string, data []byte) ([]byte, error) {
	
	if !w.Contains(account) {
		return nil, accounts.ErrUnknownAccount
	}
	
	return w.keystore.SignHashWithPassphrase(account, passphrase, crypto.Keccak256(data))
}

func (w *keystoreWallet) SignText(account accounts.Account, text []byte) ([]byte, error) {
	return w.signHash(account, accounts.TextHash(text))
}



func (w *keystoreWallet) SignTextWithPassphrase(account accounts.Account, passphrase string, text []byte) ([]byte, error) {
	
	if !w.Contains(account) {
		return nil, accounts.ErrUnknownAccount
	}
	
	return w.keystore.SignHashWithPassphrase(account, passphrase, accounts.TextHash(text))
}





func (w *keystoreWallet) SignTx(account accounts.Account, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	
	if !w.Contains(account) {
		return nil, accounts.ErrUnknownAccount
	}
	
	return w.keystore.SignTx(account, tx, chainID)
}



func (w *keystoreWallet) SignTxWithPassphrase(account accounts.Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	
	if !w.Contains(account) {
		return nil, accounts.ErrUnknownAccount
	}
	
	return w.keystore.SignTxWithPassphrase(account, passphrase, tx, chainID)
}
