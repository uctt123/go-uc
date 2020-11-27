















package snapshot

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)


type trieKV struct {
	key   common.Hash
	value []byte
}

type (
	
	
	trieGeneratorFn func(in chan (trieKV), out chan (common.Hash))

	
	
	leafCallbackFn func(hash common.Hash, stat *generateStats) common.Hash
)


func GenerateAccountTrieRoot(it AccountIterator) (common.Hash, error) {
	return generateTrieRoot(it, common.Hash{}, stdGenerate, nil, &generateStats{start: time.Now()}, true)
}


func GenerateStorageTrieRoot(account common.Hash, it StorageIterator) (common.Hash, error) {
	return generateTrieRoot(it, account, stdGenerate, nil, &generateStats{start: time.Now()}, true)
}




func VerifyState(snaptree *Tree, root common.Hash) error {
	acctIt, err := snaptree.AccountIterator(root, common.Hash{})
	if err != nil {
		return err
	}
	defer acctIt.Release()

	got, err := generateTrieRoot(acctIt, common.Hash{}, stdGenerate, func(account common.Hash, stat *generateStats) common.Hash {
		storageIt, err := snaptree.StorageIterator(root, account, common.Hash{})
		if err != nil {
			return common.Hash{}
		}
		defer storageIt.Release()

		hash, err := generateTrieRoot(storageIt, account, stdGenerate, nil, stat, false)
		if err != nil {
			return common.Hash{}
		}
		return hash
	}, &generateStats{start: time.Now()}, true)

	if err != nil {
		return err
	}
	if got != root {
		return fmt.Errorf("state root hash mismatch: got %x, want %x", got, root)
	}
	return nil
}



type generateStats struct {
	accounts   uint64
	slots      uint64
	curAccount common.Hash
	curSlot    common.Hash
	start      time.Time
	lock       sync.RWMutex
}


func (stat *generateStats) progress(accounts, slots uint64, curAccount common.Hash, curSlot common.Hash) {
	stat.lock.Lock()
	defer stat.lock.Unlock()

	stat.accounts += accounts
	stat.slots += slots
	stat.curAccount = curAccount
	stat.curSlot = curSlot
}


func (stat *generateStats) report() {
	stat.lock.RLock()
	defer stat.lock.RUnlock()

	var ctx []interface{}
	if stat.curSlot != (common.Hash{}) {
		ctx = append(ctx, []interface{}{
			"in", stat.curAccount,
			"at", stat.curSlot,
		}...)
	} else {
		ctx = append(ctx, []interface{}{"at", stat.curAccount}...)
	}
	
	ctx = append(ctx, []interface{}{"accounts", stat.accounts}...)
	if stat.slots != 0 {
		ctx = append(ctx, []interface{}{"slots", stat.slots}...)
	}
	ctx = append(ctx, []interface{}{"elapsed", common.PrettyDuration(time.Since(stat.start))}...)
	log.Info("Generating trie hash from snapshot", ctx...)
}


func (stat *generateStats) reportDone() {
	stat.lock.RLock()
	defer stat.lock.RUnlock()

	var ctx []interface{}
	ctx = append(ctx, []interface{}{"accounts", stat.accounts}...)
	if stat.slots != 0 {
		ctx = append(ctx, []interface{}{"slots", stat.slots}...)
	}
	ctx = append(ctx, []interface{}{"elapsed", common.PrettyDuration(time.Since(stat.start))}...)
	log.Info("Generated trie hash from snapshot", ctx...)
}




func generateTrieRoot(it Iterator, account common.Hash, generatorFn trieGeneratorFn, leafCallback leafCallbackFn, stats *generateStats, report bool) (common.Hash, error) {
	var (
		in      = make(chan trieKV)         
		out     = make(chan common.Hash, 1) 
		stoplog = make(chan bool, 1)        
		wg      sync.WaitGroup
	)
	
	wg.Add(1)
	go func() {
		defer wg.Done()
		generatorFn(in, out)
	}()

	
	if report && stats != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()

			timer := time.NewTimer(0)
			defer timer.Stop()

			for {
				select {
				case <-timer.C:
					stats.report()
					timer.Reset(time.Second * 8)
				case success := <-stoplog:
					if success {
						stats.reportDone()
					}
					return
				}
			}
		}()
	}
	
	
	stop := func(success bool) common.Hash {
		close(in)
		result := <-out
		stoplog <- success
		wg.Wait()
		return result
	}
	var (
		logged    = time.Now()
		processed = uint64(0)
		leaf      trieKV
		last      common.Hash
	)
	
	for it.Next() {
		if account == (common.Hash{}) {
			var (
				err      error
				fullData []byte
			)
			if leafCallback == nil {
				fullData, err = FullAccountRLP(it.(AccountIterator).Account())
				if err != nil {
					stop(false)
					return common.Hash{}, err
				}
			} else {
				account, err := FullAccount(it.(AccountIterator).Account())
				if err != nil {
					stop(false)
					return common.Hash{}, err
				}
				
				
				subroot := leafCallback(it.Hash(), stats)
				if !bytes.Equal(account.Root, subroot.Bytes()) {
					stop(false)
					return common.Hash{}, fmt.Errorf("invalid subroot(%x), want %x, got %x", it.Hash(), account.Root, subroot)
				}
				fullData, err = rlp.EncodeToBytes(account)
				if err != nil {
					stop(false)
					return common.Hash{}, err
				}
			}
			leaf = trieKV{it.Hash(), fullData}
		} else {
			leaf = trieKV{it.Hash(), common.CopyBytes(it.(StorageIterator).Slot())}
		}
		in <- leaf

		
		processed++
		if time.Since(logged) > 3*time.Second && stats != nil {
			if account == (common.Hash{}) {
				stats.progress(processed, 0, it.Hash(), common.Hash{})
			} else {
				stats.progress(0, processed, account, it.Hash())
			}
			logged, processed = time.Now(), 0
		}
		last = it.Hash()
	}
	
	if processed > 0 && stats != nil {
		if account == (common.Hash{}) {
			stats.progress(processed, 0, last, common.Hash{})
		} else {
			stats.progress(0, processed, account, last)
		}
	}
	result := stop(true)
	return result, nil
}



func stdGenerate(in chan (trieKV), out chan (common.Hash)) {
	t, _ := trie.New(common.Hash{}, trie.NewDatabase(memorydb.New()))
	for leaf := range in {
		t.TryUpdate(leaf.key[:], leaf.value)
	}
	out <- t.Hash()
}
