















package rawdb

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/ethdb/leveldb"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/olekukonko/tablewriter"
)


type freezerdb struct {
	ethdb.KeyValueStore
	ethdb.AncientStore
}



func (frdb *freezerdb) Close() error {
	var errs []error
	if err := frdb.AncientStore.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := frdb.KeyValueStore.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}




func (frdb *freezerdb) Freeze(threshold uint64) {
	
	defer func(old uint64) {
		atomic.StoreUint64(&frdb.AncientStore.(*freezer).threshold, old)
	}(atomic.LoadUint64(&frdb.AncientStore.(*freezer).threshold))
	atomic.StoreUint64(&frdb.AncientStore.(*freezer).threshold, threshold)

	
	trigger := make(chan struct{}, 1)
	frdb.AncientStore.(*freezer).trigger <- trigger
	<-trigger
}


type nofreezedb struct {
	ethdb.KeyValueStore
}


func (db *nofreezedb) HasAncient(kind string, number uint64) (bool, error) {
	return false, errNotSupported
}


func (db *nofreezedb) Ancient(kind string, number uint64) ([]byte, error) {
	return nil, errNotSupported
}


func (db *nofreezedb) Ancients() (uint64, error) {
	return 0, errNotSupported
}


func (db *nofreezedb) AncientSize(kind string) (uint64, error) {
	return 0, errNotSupported
}


func (db *nofreezedb) AppendAncient(number uint64, hash, header, body, receipts, td []byte) error {
	return errNotSupported
}


func (db *nofreezedb) TruncateAncients(items uint64) error {
	return errNotSupported
}


func (db *nofreezedb) Sync() error {
	return errNotSupported
}



func NewDatabase(db ethdb.KeyValueStore) ethdb.Database {
	return &nofreezedb{
		KeyValueStore: db,
	}
}




func NewDatabaseWithFreezer(db ethdb.KeyValueStore, freezer string, namespace string) (ethdb.Database, error) {
	
	frdb, err := newFreezer(freezer, namespace)
	if err != nil {
		return nil, err
	}
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	

	
	
	
	if kvgenesis, _ := db.Get(headerHashKey(0)); len(kvgenesis) > 0 {
		if frozen, _ := frdb.Ancients(); frozen > 0 {
			
			
			
			frgenesis, err := frdb.Ancient(freezerHashTable, 0)
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve genesis from ancient %v", err)
			} else if !bytes.Equal(kvgenesis, frgenesis) {
				return nil, fmt.Errorf("genesis mismatch: %#x (leveldb) != %#x (ancients)", kvgenesis, frgenesis)
			}
			
			
			if kvhash, _ := db.Get(headerHashKey(frozen)); len(kvhash) == 0 {
				
				
				if *ReadHeaderNumber(db, ReadHeadHeaderHash(db)) > frozen-1 {
					return nil, fmt.Errorf("gap (#%d) in the chain between ancients and leveldb", frozen)
				}
				
				
			}
			
			
			
		} else {
			
			
			
			
			if ReadHeadHeaderHash(db) != common.BytesToHash(kvgenesis) {
				
				
				if kvblob, _ := db.Get(headerHashKey(1)); len(kvblob) == 0 {
					return nil, errors.New("ancient chain segments already extracted, please set --datadir.ancient to the correct path")
				}
				
			}
			
			
		}
	}
	
	go frdb.freeze(db)

	return &freezerdb{
		KeyValueStore: db,
		AncientStore:  frdb,
	}, nil
}



func NewMemoryDatabase() ethdb.Database {
	return NewDatabase(memorydb.New())
}




func NewMemoryDatabaseWithCap(size int) ethdb.Database {
	return NewDatabase(memorydb.NewWithCap(size))
}



func NewLevelDBDatabase(file string, cache int, handles int, namespace string) (ethdb.Database, error) {
	db, err := leveldb.New(file, cache, handles, namespace)
	if err != nil {
		return nil, err
	}
	return NewDatabase(db), nil
}



func NewLevelDBDatabaseWithFreezer(file string, cache int, handles int, freezer string, namespace string) (ethdb.Database, error) {
	kvdb, err := leveldb.New(file, cache, handles, namespace)
	if err != nil {
		return nil, err
	}
	frdb, err := NewDatabaseWithFreezer(kvdb, freezer, namespace)
	if err != nil {
		kvdb.Close()
		return nil, err
	}
	return frdb, nil
}

type counter uint64

func (c counter) String() string {
	return fmt.Sprintf("%d", c)
}

func (c counter) Percentage(current uint64) string {
	return fmt.Sprintf("%d", current*100/uint64(c))
}


type stat struct {
	size  common.StorageSize
	count counter
}


func (s *stat) Add(size common.StorageSize) {
	s.size += size
	s.count++
}

func (s *stat) Size() string {
	return s.size.String()
}

func (s *stat) Count() string {
	return s.count.String()
}



func InspectDatabase(db ethdb.Database) error {
	it := db.NewIterator(nil, nil)
	defer it.Release()

	var (
		count  int64
		start  = time.Now()
		logged = time.Now()

		
		headers         stat
		bodies          stat
		receipts        stat
		tds             stat
		numHashPairings stat
		hashNumPairings stat
		tries           stat
		codes           stat
		txLookups       stat
		accountSnaps    stat
		storageSnaps    stat
		preimages       stat
		bloomBits       stat
		cliqueSnaps     stat

		
		ancientHeadersSize  common.StorageSize
		ancientBodiesSize   common.StorageSize
		ancientReceiptsSize common.StorageSize
		ancientTdsSize      common.StorageSize
		ancientHashesSize   common.StorageSize

		
		chtTrieNodes   stat
		bloomTrieNodes stat

		
		metadata    stat
		unaccounted stat

		
		total common.StorageSize
	)
	
	for it.Next() {
		var (
			key  = it.Key()
			size = common.StorageSize(len(key) + len(it.Value()))
		)
		total += size
		switch {
		case bytes.HasPrefix(key, headerPrefix) && len(key) == (len(headerPrefix)+8+common.HashLength):
			headers.Add(size)
		case bytes.HasPrefix(key, blockBodyPrefix) && len(key) == (len(blockBodyPrefix)+8+common.HashLength):
			bodies.Add(size)
		case bytes.HasPrefix(key, blockReceiptsPrefix) && len(key) == (len(blockReceiptsPrefix)+8+common.HashLength):
			receipts.Add(size)
		case bytes.HasPrefix(key, headerPrefix) && bytes.HasSuffix(key, headerTDSuffix):
			tds.Add(size)
		case bytes.HasPrefix(key, headerPrefix) && bytes.HasSuffix(key, headerHashSuffix):
			numHashPairings.Add(size)
		case bytes.HasPrefix(key, headerNumberPrefix) && len(key) == (len(headerNumberPrefix)+common.HashLength):
			hashNumPairings.Add(size)
		case len(key) == common.HashLength:
			tries.Add(size)
		case bytes.HasPrefix(key, codePrefix) && len(key) == len(codePrefix)+common.HashLength:
			codes.Add(size)
		case bytes.HasPrefix(key, txLookupPrefix) && len(key) == (len(txLookupPrefix)+common.HashLength):
			txLookups.Add(size)
		case bytes.HasPrefix(key, SnapshotAccountPrefix) && len(key) == (len(SnapshotAccountPrefix)+common.HashLength):
			accountSnaps.Add(size)
		case bytes.HasPrefix(key, SnapshotStoragePrefix) && len(key) == (len(SnapshotStoragePrefix)+2*common.HashLength):
			storageSnaps.Add(size)
		case bytes.HasPrefix(key, preimagePrefix) && len(key) == (len(preimagePrefix)+common.HashLength):
			preimages.Add(size)
		case bytes.HasPrefix(key, bloomBitsPrefix) && len(key) == (len(bloomBitsPrefix)+10+common.HashLength):
			bloomBits.Add(size)
		case bytes.HasPrefix(key, []byte("clique-")) && len(key) == 7+common.HashLength:
			cliqueSnaps.Add(size)
		case bytes.HasPrefix(key, []byte("cht-")) && len(key) == 4+common.HashLength:
			chtTrieNodes.Add(size)
		case bytes.HasPrefix(key, []byte("blt-")) && len(key) == 4+common.HashLength:
			bloomTrieNodes.Add(size)
		default:
			var accounted bool
			for _, meta := range [][]byte{databaseVerisionKey, headHeaderKey, headBlockKey, headFastBlockKey, fastTrieProgressKey} {
				if bytes.Equal(key, meta) {
					metadata.Add(size)
					accounted = true
					break
				}
			}
			if !accounted {
				unaccounted.Add(size)
			}
		}
		count++
		if count%1000 == 0 && time.Since(logged) > 8*time.Second {
			log.Info("Inspecting database", "count", count, "elapsed", common.PrettyDuration(time.Since(start)))
			logged = time.Now()
		}
	}
	
	ancientSizes := []*common.StorageSize{&ancientHeadersSize, &ancientBodiesSize, &ancientReceiptsSize, &ancientHashesSize, &ancientTdsSize}
	for i, category := range []string{freezerHeaderTable, freezerBodiesTable, freezerReceiptTable, freezerHashTable, freezerDifficultyTable} {
		if size, err := db.AncientSize(category); err == nil {
			*ancientSizes[i] += common.StorageSize(size)
			total += common.StorageSize(size)
		}
	}
	
	ancients := counter(0)
	if count, err := db.Ancients(); err == nil {
		ancients = counter(count)
	}
	
	stats := [][]string{
		{"Key-Value store", "Headers", headers.Size(), headers.Count()},
		{"Key-Value store", "Bodies", bodies.Size(), bodies.Count()},
		{"Key-Value store", "Receipt lists", receipts.Size(), receipts.Count()},
		{"Key-Value store", "Difficulties", tds.Size(), tds.Count()},
		{"Key-Value store", "Block number->hash", numHashPairings.Size(), numHashPairings.Count()},
		{"Key-Value store", "Block hash->number", hashNumPairings.Size(), hashNumPairings.Count()},
		{"Key-Value store", "Transaction index", txLookups.Size(), txLookups.Count()},
		{"Key-Value store", "Bloombit index", bloomBits.Size(), bloomBits.Count()},
		{"Key-Value store", "Contract codes", codes.Size(), codes.Count()},
		{"Key-Value store", "Trie nodes", tries.Size(), tries.Count()},
		{"Key-Value store", "Trie preimages", preimages.Size(), preimages.Count()},
		{"Key-Value store", "Account snapshot", accountSnaps.Size(), accountSnaps.Count()},
		{"Key-Value store", "Storage snapshot", storageSnaps.Size(), storageSnaps.Count()},
		{"Key-Value store", "Clique snapshots", cliqueSnaps.Size(), cliqueSnaps.Count()},
		{"Key-Value store", "Singleton metadata", metadata.Size(), metadata.Count()},
		{"Ancient store", "Headers", ancientHeadersSize.String(), ancients.String()},
		{"Ancient store", "Bodies", ancientBodiesSize.String(), ancients.String()},
		{"Ancient store", "Receipt lists", ancientReceiptsSize.String(), ancients.String()},
		{"Ancient store", "Difficulties", ancientTdsSize.String(), ancients.String()},
		{"Ancient store", "Block number->hash", ancientHashesSize.String(), ancients.String()},
		{"Light client", "CHT trie nodes", chtTrieNodes.Size(), chtTrieNodes.Count()},
		{"Light client", "Bloom trie nodes", bloomTrieNodes.Size(), bloomTrieNodes.Count()},
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Database", "Category", "Size", "Items"})
	table.SetFooter([]string{"", "Total", total.String(), " "})
	table.AppendBulk(stats)
	table.Render()

	if unaccounted.size > 0 {
		log.Error("Database contains unaccounted data", "size", unaccounted.size, "count", unaccounted.count)
	}

	return nil
}
