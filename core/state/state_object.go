















package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/rlp"
)

var emptyCodeHash = crypto.Keccak256(nil)

type Code []byte

func (c Code) String() string {
	return string(c) 
}

type Storage map[common.Hash]common.Hash

func (s Storage) String() (str string) {
	for key, value := range s {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}

	return cpy
}







type stateObject struct {
	address  common.Address
	addrHash common.Hash 
	data     Account
	db       *StateDB

	
	
	
	
	
	dbErr error

	
	trie Trie 
	code Code 

	originStorage  Storage 
	pendingStorage Storage 
	dirtyStorage   Storage 
	fakeStorage    Storage 

	
	
	
	dirtyCode bool 
	suicided  bool
	deleted   bool
}


func (s *stateObject) empty() bool {
	return s.data.Nonce == 0 && s.data.Balance.Sign() == 0 && bytes.Equal(s.data.CodeHash, emptyCodeHash)
}



type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash 
	CodeHash []byte
}


func newObject(db *StateDB, address common.Address, data Account) *stateObject {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}
	if data.CodeHash == nil {
		data.CodeHash = emptyCodeHash
	}
	if data.Root == (common.Hash{}) {
		data.Root = emptyRoot
	}
	return &stateObject{
		db:             db,
		address:        address,
		addrHash:       crypto.Keccak256Hash(address[:]),
		data:           data,
		originStorage:  make(Storage),
		pendingStorage: make(Storage),
		dirtyStorage:   make(Storage),
	}
}


func (s *stateObject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}


func (s *stateObject) setError(err error) {
	if s.dbErr == nil {
		s.dbErr = err
	}
}

func (s *stateObject) markSuicided() {
	s.suicided = true
}

func (s *stateObject) touch() {
	s.db.journal.append(touchChange{
		account: &s.address,
	})
	if s.address == ripemd {
		
		
		s.db.journal.dirty(s.address)
	}
}

func (s *stateObject) getTrie(db Database) Trie {
	if s.trie == nil {
		var err error
		s.trie, err = db.OpenStorageTrie(s.addrHash, s.data.Root)
		if err != nil {
			s.trie, _ = db.OpenStorageTrie(s.addrHash, common.Hash{})
			s.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return s.trie
}


func (s *stateObject) GetState(db Database, key common.Hash) common.Hash {
	
	if s.fakeStorage != nil {
		return s.fakeStorage[key]
	}
	
	value, dirty := s.dirtyStorage[key]
	if dirty {
		return value
	}
	
	return s.GetCommittedState(db, key)
}


func (s *stateObject) GetCommittedState(db Database, key common.Hash) common.Hash {
	
	if s.fakeStorage != nil {
		return s.fakeStorage[key]
	}
	
	if value, pending := s.pendingStorage[key]; pending {
		return value
	}
	if value, cached := s.originStorage[key]; cached {
		return value
	}
	
	var (
		enc []byte
		err error
	)
	if s.db.snap != nil {
		if metrics.EnabledExpensive {
			defer func(start time.Time) { s.db.SnapshotStorageReads += time.Since(start) }(time.Now())
		}
		
		
		
		
		
		
		if _, destructed := s.db.snapDestructs[s.addrHash]; destructed {
			return common.Hash{}
		}
		enc, err = s.db.snap.Storage(s.addrHash, crypto.Keccak256Hash(key.Bytes()))
	}
	
	if s.db.snap == nil || err != nil {
		if metrics.EnabledExpensive {
			defer func(start time.Time) { s.db.StorageReads += time.Since(start) }(time.Now())
		}
		if enc, err = s.getTrie(db).TryGet(key.Bytes()); err != nil {
			s.setError(err)
			return common.Hash{}
		}
	}
	var value common.Hash
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			s.setError(err)
		}
		value.SetBytes(content)
	}
	s.originStorage[key] = value
	return value
}


func (s *stateObject) SetState(db Database, key, value common.Hash) {
	
	if s.fakeStorage != nil {
		s.fakeStorage[key] = value
		return
	}
	
	prev := s.GetState(db, key)
	if prev == value {
		return
	}
	
	s.db.journal.append(storageChange{
		account:  &s.address,
		key:      key,
		prevalue: prev,
	})
	s.setState(key, value)
}







func (s *stateObject) SetStorage(storage map[common.Hash]common.Hash) {
	
	if s.fakeStorage == nil {
		s.fakeStorage = make(Storage)
	}
	for key, value := range storage {
		s.fakeStorage[key] = value
	}
	
	
}

func (s *stateObject) setState(key, value common.Hash) {
	s.dirtyStorage[key] = value
}



func (s *stateObject) finalise() {
	for key, value := range s.dirtyStorage {
		s.pendingStorage[key] = value
	}
	if len(s.dirtyStorage) > 0 {
		s.dirtyStorage = make(Storage)
	}
}



func (s *stateObject) updateTrie(db Database) Trie {
	
	s.finalise()
	if len(s.pendingStorage) == 0 {
		return s.trie
	}
	
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageUpdates += time.Since(start) }(time.Now())
	}
	
	var storage map[common.Hash][]byte
	if s.db.snap != nil {
		
		storage = s.db.snapStorage[s.addrHash]
		if storage == nil {
			storage = make(map[common.Hash][]byte)
			s.db.snapStorage[s.addrHash] = storage
		}
	}
	
	tr := s.getTrie(db)
	for key, value := range s.pendingStorage {
		
		if value == s.originStorage[key] {
			continue
		}
		s.originStorage[key] = value

		var v []byte
		if (value == common.Hash{}) {
			s.setError(tr.TryDelete(key[:]))
		} else {
			
			v, _ = rlp.EncodeToBytes(common.TrimLeftZeroes(value[:]))
			s.setError(tr.TryUpdate(key[:], v))
		}
		
		if storage != nil {
			storage[crypto.Keccak256Hash(key[:])] = v 
		}
	}
	if len(s.pendingStorage) > 0 {
		s.pendingStorage = make(Storage)
	}
	return tr
}


func (s *stateObject) updateRoot(db Database) {
	
	if s.updateTrie(db) == nil {
		return
	}
	
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageHashes += time.Since(start) }(time.Now())
	}
	s.data.Root = s.trie.Hash()
}



func (s *stateObject) CommitTrie(db Database) error {
	
	if s.updateTrie(db) == nil {
		return nil
	}
	if s.dbErr != nil {
		return s.dbErr
	}
	
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageCommits += time.Since(start) }(time.Now())
	}
	root, err := s.trie.Commit(nil)
	if err == nil {
		s.data.Root = root
	}
	return err
}



func (s *stateObject) AddBalance(amount *big.Int) {
	
	
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}
		return
	}
	s.SetBalance(new(big.Int).Add(s.Balance(), amount))
}



func (s *stateObject) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	s.SetBalance(new(big.Int).Sub(s.Balance(), amount))
}

func (s *stateObject) SetBalance(amount *big.Int) {
	s.db.journal.append(balanceChange{
		account: &s.address,
		prev:    new(big.Int).Set(s.data.Balance),
	})
	s.setBalance(amount)
}

func (s *stateObject) setBalance(amount *big.Int) {
	s.data.Balance = amount
}


func (s *stateObject) ReturnGas(gas *big.Int) {}

func (s *stateObject) deepCopy(db *StateDB) *stateObject {
	stateObject := newObject(db, s.address, s.data)
	if s.trie != nil {
		stateObject.trie = db.db.CopyTrie(s.trie)
	}
	stateObject.code = s.code
	stateObject.dirtyStorage = s.dirtyStorage.Copy()
	stateObject.originStorage = s.originStorage.Copy()
	stateObject.pendingStorage = s.pendingStorage.Copy()
	stateObject.suicided = s.suicided
	stateObject.dirtyCode = s.dirtyCode
	stateObject.deleted = s.deleted
	return stateObject
}






func (s *stateObject) Address() common.Address {
	return s.address
}


func (s *stateObject) Code(db Database) []byte {
	if s.code != nil {
		return s.code
	}
	if bytes.Equal(s.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(s.addrHash, common.BytesToHash(s.CodeHash()))
	if err != nil {
		s.setError(fmt.Errorf("can't load code hash %x: %v", s.CodeHash(), err))
	}
	s.code = code
	return code
}




func (s *stateObject) CodeSize(db Database) int {
	if s.code != nil {
		return len(s.code)
	}
	if bytes.Equal(s.CodeHash(), emptyCodeHash) {
		return 0
	}
	size, err := db.ContractCodeSize(s.addrHash, common.BytesToHash(s.CodeHash()))
	if err != nil {
		s.setError(fmt.Errorf("can't load code size %x: %v", s.CodeHash(), err))
	}
	return size
}

func (s *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := s.Code(s.db.db)
	s.db.journal.append(codeChange{
		account:  &s.address,
		prevhash: s.CodeHash(),
		prevcode: prevcode,
	})
	s.setCode(codeHash, code)
}

func (s *stateObject) setCode(codeHash common.Hash, code []byte) {
	s.code = code
	s.data.CodeHash = codeHash[:]
	s.dirtyCode = true
}

func (s *stateObject) SetNonce(nonce uint64) {
	s.db.journal.append(nonceChange{
		account: &s.address,
		prev:    s.data.Nonce,
	})
	s.setNonce(nonce)
}

func (s *stateObject) setNonce(nonce uint64) {
	s.data.Nonce = nonce
}

func (s *stateObject) CodeHash() []byte {
	return s.data.CodeHash
}

func (s *stateObject) Balance() *big.Int {
	return s.data.Balance
}

func (s *stateObject) Nonce() uint64 {
	return s.data.Nonce
}




func (s *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}
