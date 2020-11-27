















package state

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)


type DumpCollector interface {
	
	OnRoot(common.Hash)
	
	OnAccount(common.Address, DumpAccount)
}


type DumpAccount struct {
	Balance   string                 `json:"balance"`
	Nonce     uint64                 `json:"nonce"`
	Root      string                 `json:"root"`
	CodeHash  string                 `json:"codeHash"`
	Code      string                 `json:"code,omitempty"`
	Storage   map[common.Hash]string `json:"storage,omitempty"`
	Address   *common.Address        `json:"address,omitempty"` 
	SecureKey hexutil.Bytes          `json:"key,omitempty"`     

}


type Dump struct {
	Root     string                         `json:"root"`
	Accounts map[common.Address]DumpAccount `json:"accounts"`
}


func (d *Dump) OnRoot(root common.Hash) {
	d.Root = fmt.Sprintf("%x", root)
}


func (d *Dump) OnAccount(addr common.Address, account DumpAccount) {
	d.Accounts[addr] = account
}


type IteratorDump struct {
	Root     string                         `json:"root"`
	Accounts map[common.Address]DumpAccount `json:"accounts"`
	Next     []byte                         `json:"next,omitempty"` 
}


func (d *IteratorDump) OnRoot(root common.Hash) {
	d.Root = fmt.Sprintf("%x", root)
}


func (d *IteratorDump) OnAccount(addr common.Address, account DumpAccount) {
	d.Accounts[addr] = account
}


type iterativeDump struct {
	*json.Encoder
}


func (d iterativeDump) OnAccount(addr common.Address, account DumpAccount) {
	dumpAccount := &DumpAccount{
		Balance:   account.Balance,
		Nonce:     account.Nonce,
		Root:      account.Root,
		CodeHash:  account.CodeHash,
		Code:      account.Code,
		Storage:   account.Storage,
		SecureKey: account.SecureKey,
		Address:   nil,
	}
	if addr != (common.Address{}) {
		dumpAccount.Address = &addr
	}
	d.Encode(dumpAccount)
}


func (d iterativeDump) OnRoot(root common.Hash) {
	d.Encode(struct {
		Root common.Hash `json:"root"`
	}{root})
}

func (s *StateDB) DumpToCollector(c DumpCollector, excludeCode, excludeStorage, excludeMissingPreimages bool, start []byte, maxResults int) (nextKey []byte) {
	missingPreimages := 0
	c.OnRoot(s.trie.Hash())

	var count int
	it := trie.NewIterator(s.trie.NodeIterator(start))
	for it.Next() {
		var data Account
		if err := rlp.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}
		account := DumpAccount{
			Balance:  data.Balance.String(),
			Nonce:    data.Nonce,
			Root:     common.Bytes2Hex(data.Root[:]),
			CodeHash: common.Bytes2Hex(data.CodeHash),
		}
		addrBytes := s.trie.GetKey(it.Key)
		if addrBytes == nil {
			
			missingPreimages++
			if excludeMissingPreimages {
				continue
			}
			account.SecureKey = it.Key
		}
		addr := common.BytesToAddress(addrBytes)
		obj := newObject(nil, addr, data)
		if !excludeCode {
			account.Code = common.Bytes2Hex(obj.Code(s.db))
		}
		if !excludeStorage {
			account.Storage = make(map[common.Hash]string)
			storageIt := trie.NewIterator(obj.getTrie(s.db).NodeIterator(nil))
			for storageIt.Next() {
				_, content, _, err := rlp.Split(storageIt.Value)
				if err != nil {
					log.Error("Failed to decode the value returned by iterator", "error", err)
					continue
				}
				account.Storage[common.BytesToHash(s.trie.GetKey(storageIt.Key))] = common.Bytes2Hex(content)
			}
		}
		c.OnAccount(addr, account)
		count++
		if maxResults > 0 && count >= maxResults {
			if it.Next() {
				nextKey = it.Key
			}
			break
		}
	}
	if missingPreimages > 0 {
		log.Warn("Dump incomplete due to missing preimages", "missing", missingPreimages)
	}

	return nextKey
}


func (s *StateDB) RawDump(excludeCode, excludeStorage, excludeMissingPreimages bool) Dump {
	dump := &Dump{
		Accounts: make(map[common.Address]DumpAccount),
	}
	s.DumpToCollector(dump, excludeCode, excludeStorage, excludeMissingPreimages, nil, 0)
	return *dump
}


func (s *StateDB) Dump(excludeCode, excludeStorage, excludeMissingPreimages bool) []byte {
	dump := s.RawDump(excludeCode, excludeStorage, excludeMissingPreimages)
	json, err := json.MarshalIndent(dump, "", "    ")
	if err != nil {
		fmt.Println("Dump err", err)
	}
	return json
}


func (s *StateDB) IterativeDump(excludeCode, excludeStorage, excludeMissingPreimages bool, output *json.Encoder) {
	s.DumpToCollector(iterativeDump{output}, excludeCode, excludeStorage, excludeMissingPreimages, nil, 0)
}


func (s *StateDB) IteratorDump(excludeCode, excludeStorage, excludeMissingPreimages bool, start []byte, maxResults int) IteratorDump {
	iterator := &IteratorDump{
		Accounts: make(map[common.Address]DumpAccount),
	}
	iterator.Next = s.DumpToCollector(iterator, excludeCode, excludeStorage, excludeMissingPreimages, start, maxResults)
	return *iterator
}
