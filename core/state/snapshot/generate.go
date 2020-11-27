















package snapshot

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

var (
	
	emptyRoot = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	
	emptyCode = crypto.Keccak256Hash(nil)
)



type generatorStats struct {
	wiping   chan struct{}      
	origin   uint64             
	start    time.Time          
	accounts uint64             
	slots    uint64             
	storage  common.StorageSize 
}



func (gs *generatorStats) Log(msg string, root common.Hash, marker []byte) {
	var ctx []interface{}
	if root != (common.Hash{}) {
		ctx = append(ctx, []interface{}{"root", root}...)
	}
	
	switch len(marker) {
	case common.HashLength:
		ctx = append(ctx, []interface{}{"at", common.BytesToHash(marker)}...)
	case 2 * common.HashLength:
		ctx = append(ctx, []interface{}{
			"in", common.BytesToHash(marker[:common.HashLength]),
			"at", common.BytesToHash(marker[common.HashLength:]),
		}...)
	}
	
	ctx = append(ctx, []interface{}{
		"accounts", gs.accounts,
		"slots", gs.slots,
		"storage", gs.storage,
		"elapsed", common.PrettyDuration(time.Since(gs.start)),
	}...)
	
	if len(marker) > 0 {
		if done := binary.BigEndian.Uint64(marker[:8]) - gs.origin; done > 0 {
			left := math.MaxUint64 - binary.BigEndian.Uint64(marker[:8])

			speed := done/uint64(time.Since(gs.start)/time.Millisecond+1) + 1 
			ctx = append(ctx, []interface{}{
				"eta", common.PrettyDuration(time.Duration(left/speed) * time.Millisecond),
			}...)
		}
	}
	log.Info(msg, ctx...)
}




func generateSnapshot(diskdb ethdb.KeyValueStore, triedb *trie.Database, cache int, root common.Hash, wiper chan struct{}) *diskLayer {
	
	
	if wiper == nil {
		wiper = wipeSnapshot(diskdb, true)
	}
	
	rawdb.WriteSnapshotRoot(diskdb, root)

	base := &diskLayer{
		diskdb:     diskdb,
		triedb:     triedb,
		root:       root,
		cache:      fastcache.New(cache * 1024 * 1024),
		genMarker:  []byte{}, 
		genPending: make(chan struct{}),
		genAbort:   make(chan chan *generatorStats),
	}
	go base.generate(&generatorStats{wiping: wiper, start: time.Now()})
	return base
}





func (dl *diskLayer) generate(stats *generatorStats) {
	
	if stats.wiping != nil {
		stats.Log("Wiper running, state snapshotting paused", common.Hash{}, dl.genMarker)
		select {
		
		case <-stats.wiping:
			stats.wiping = nil
			stats.start = time.Now()

		
		case abort := <-dl.genAbort:
			abort <- stats
			return
		}
	}
	
	accTrie, err := trie.NewSecure(dl.root, dl.triedb)
	if err != nil {
		
		stats.Log("Trie missing, state snapshotting paused", dl.root, dl.genMarker)

		abort := <-dl.genAbort
		abort <- stats
		return
	}
	stats.Log("Resuming state snapshot generation", dl.root, dl.genMarker)

	var accMarker []byte
	if len(dl.genMarker) > 0 { 
		accMarker = dl.genMarker[:common.HashLength]
	}
	accIt := trie.NewIterator(accTrie.NodeIterator(accMarker))
	batch := dl.diskdb.NewBatch()

	
	logged := time.Now()
	for accIt.Next() {
		
		accountHash := common.BytesToHash(accIt.Key)

		var acc struct {
			Nonce    uint64
			Balance  *big.Int
			Root     common.Hash
			CodeHash []byte
		}
		if err := rlp.DecodeBytes(accIt.Value, &acc); err != nil {
			log.Crit("Invalid account encountered during snapshot creation", "err", err)
		}
		data := SlimAccountRLP(acc.Nonce, acc.Balance, acc.Root, acc.CodeHash)

		
		if accMarker == nil || !bytes.Equal(accountHash[:], accMarker) {
			rawdb.WriteAccountSnapshot(batch, accountHash, data)
			stats.storage += common.StorageSize(1 + common.HashLength + len(data))
			stats.accounts++
		}
		
		var abort chan *generatorStats
		select {
		case abort = <-dl.genAbort:
		default:
		}
		if batch.ValueSize() > ethdb.IdealBatchSize || abort != nil {
			
			if batch.ValueSize() > 0 {
				batch.Write()
				batch.Reset()

				dl.lock.Lock()
				dl.genMarker = accountHash[:]
				dl.lock.Unlock()
			}
			if abort != nil {
				stats.Log("Aborting state snapshot generation", dl.root, accountHash[:])
				abort <- stats
				return
			}
		}
		
		if acc.Root != emptyRoot {
			storeTrie, err := trie.NewSecure(acc.Root, dl.triedb)
			if err != nil {
				log.Error("Generator failed to access storage trie", "accroot", dl.root, "acchash", common.BytesToHash(accIt.Key), "stroot", acc.Root, "err", err)
				abort := <-dl.genAbort
				abort <- stats
				return
			}
			var storeMarker []byte
			if accMarker != nil && bytes.Equal(accountHash[:], accMarker) && len(dl.genMarker) > common.HashLength {
				storeMarker = dl.genMarker[common.HashLength:]
			}
			storeIt := trie.NewIterator(storeTrie.NodeIterator(storeMarker))
			for storeIt.Next() {
				rawdb.WriteStorageSnapshot(batch, accountHash, common.BytesToHash(storeIt.Key), storeIt.Value)
				stats.storage += common.StorageSize(1 + 2*common.HashLength + len(storeIt.Value))
				stats.slots++

				
				var abort chan *generatorStats
				select {
				case abort = <-dl.genAbort:
				default:
				}
				if batch.ValueSize() > ethdb.IdealBatchSize || abort != nil {
					
					if batch.ValueSize() > 0 {
						batch.Write()
						batch.Reset()

						dl.lock.Lock()
						dl.genMarker = append(accountHash[:], storeIt.Key...)
						dl.lock.Unlock()
					}
					if abort != nil {
						stats.Log("Aborting state snapshot generation", dl.root, append(accountHash[:], storeIt.Key...))
						abort <- stats
						return
					}
				}
			}
			if err := storeIt.Err; err != nil {
				log.Error("Generator failed to iterate storage trie", "accroot", dl.root, "acchash", common.BytesToHash(accIt.Key), "stroot", acc.Root, "err", err)
				abort := <-dl.genAbort
				abort <- stats
				return
			}
		}
		if time.Since(logged) > 8*time.Second {
			stats.Log("Generating state snapshot", dl.root, accIt.Key)
			logged = time.Now()
		}
		
		accMarker = nil
	}
	if err := accIt.Err; err != nil {
		log.Error("Generator failed to iterate account trie", "root", dl.root, "err", err)
		abort := <-dl.genAbort
		abort <- stats
		return
	}
	
	if batch.ValueSize() > 0 {
		batch.Write()
	}
	log.Info("Generated state snapshot", "accounts", stats.accounts, "slots", stats.slots,
		"storage", stats.storage, "elapsed", common.PrettyDuration(time.Since(stats.start)))

	dl.lock.Lock()
	dl.genMarker = nil
	close(dl.genPending)
	dl.lock.Unlock()

	
	abort := <-dl.genAbort
	abort <- nil
}
