















package rawdb

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/params"
	"github.com/prometheus/tsdb/fileutil"
)

var (
	
	
	errUnknownTable = errors.New("unknown table")

	
	
	errOutOrderInsertion = errors.New("the append operation is out-order")

	
	
	errSymlinkDatadir = errors.New("symbolic link datadir is not supported")
)

const (
	
	
	
	freezerRecheckInterval = time.Minute

	
	
	freezerBatchLimit = 30000
)








type freezer struct {
	
	
	
	frozen    uint64 
	threshold uint64 

	tables       map[string]*freezerTable 
	instanceLock fileutil.Releaser        

	trigger chan chan struct{} 

	quit      chan struct{}
	closeOnce sync.Once
}



func newFreezer(datadir string, namespace string) (*freezer, error) {
	
	var (
		readMeter  = metrics.NewRegisteredMeter(namespace+"ancient/read", nil)
		writeMeter = metrics.NewRegisteredMeter(namespace+"ancient/write", nil)
		sizeGauge  = metrics.NewRegisteredGauge(namespace+"ancient/size", nil)
	)
	
	if info, err := os.Lstat(datadir); !os.IsNotExist(err) {
		if info.Mode()&os.ModeSymlink != 0 {
			log.Warn("Symbolic link ancient database is not supported", "path", datadir)
			return nil, errSymlinkDatadir
		}
	}
	
	
	lock, _, err := fileutil.Flock(filepath.Join(datadir, "FLOCK"))
	if err != nil {
		return nil, err
	}
	
	freezer := &freezer{
		threshold:    params.FullImmutabilityThreshold,
		tables:       make(map[string]*freezerTable),
		instanceLock: lock,
		trigger:      make(chan chan struct{}),
		quit:         make(chan struct{}),
	}
	for name, disableSnappy := range freezerNoSnappy {
		table, err := newTable(datadir, name, readMeter, writeMeter, sizeGauge, disableSnappy)
		if err != nil {
			for _, table := range freezer.tables {
				table.Close()
			}
			lock.Release()
			return nil, err
		}
		freezer.tables[name] = table
	}
	if err := freezer.repair(); err != nil {
		for _, table := range freezer.tables {
			table.Close()
		}
		lock.Release()
		return nil, err
	}
	log.Info("Opened ancient database", "database", datadir)
	return freezer, nil
}


func (f *freezer) Close() error {
	var errs []error
	f.closeOnce.Do(func() {
		f.quit <- struct{}{}
		for _, table := range f.tables {
			if err := table.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if err := f.instanceLock.Release(); err != nil {
			errs = append(errs, err)
		}
	})
	if errs != nil {
		return fmt.Errorf("%v", errs)
	}
	return nil
}



func (f *freezer) HasAncient(kind string, number uint64) (bool, error) {
	if table := f.tables[kind]; table != nil {
		return table.has(number), nil
	}
	return false, nil
}


func (f *freezer) Ancient(kind string, number uint64) ([]byte, error) {
	if table := f.tables[kind]; table != nil {
		return table.Retrieve(number)
	}
	return nil, errUnknownTable
}


func (f *freezer) Ancients() (uint64, error) {
	return atomic.LoadUint64(&f.frozen), nil
}


func (f *freezer) AncientSize(kind string) (uint64, error) {
	if table := f.tables[kind]; table != nil {
		return table.size()
	}
	return 0, errUnknownTable
}







func (f *freezer) AppendAncient(number uint64, hash, header, body, receipts, td []byte) (err error) {
	
	if atomic.LoadUint64(&f.frozen) != number {
		return errOutOrderInsertion
	}
	
	
	defer func() {
		if err != nil {
			rerr := f.repair()
			if rerr != nil {
				log.Crit("Failed to repair freezer", "err", rerr)
			}
			log.Info("Append ancient failed", "number", number, "err", err)
		}
	}()
	
	if err := f.tables[freezerHashTable].Append(f.frozen, hash[:]); err != nil {
		log.Error("Failed to append ancient hash", "number", f.frozen, "hash", hash, "err", err)
		return err
	}
	if err := f.tables[freezerHeaderTable].Append(f.frozen, header); err != nil {
		log.Error("Failed to append ancient header", "number", f.frozen, "hash", hash, "err", err)
		return err
	}
	if err := f.tables[freezerBodiesTable].Append(f.frozen, body); err != nil {
		log.Error("Failed to append ancient body", "number", f.frozen, "hash", hash, "err", err)
		return err
	}
	if err := f.tables[freezerReceiptTable].Append(f.frozen, receipts); err != nil {
		log.Error("Failed to append ancient receipts", "number", f.frozen, "hash", hash, "err", err)
		return err
	}
	if err := f.tables[freezerDifficultyTable].Append(f.frozen, td); err != nil {
		log.Error("Failed to append ancient difficulty", "number", f.frozen, "hash", hash, "err", err)
		return err
	}
	atomic.AddUint64(&f.frozen, 1) 
	return nil
}


func (f *freezer) TruncateAncients(items uint64) error {
	if atomic.LoadUint64(&f.frozen) <= items {
		return nil
	}
	for _, table := range f.tables {
		if err := table.truncate(items); err != nil {
			return err
		}
	}
	atomic.StoreUint64(&f.frozen, items)
	return nil
}


func (f *freezer) Sync() error {
	var errs []error
	for _, table := range f.tables {
		if err := table.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	if errs != nil {
		return fmt.Errorf("%v", errs)
	}
	return nil
}






func (f *freezer) freeze(db ethdb.KeyValueStore) {
	nfdb := &nofreezedb{KeyValueStore: db}

	var (
		backoff   bool
		triggered chan struct{} 
	)
	for {
		select {
		case <-f.quit:
			log.Info("Freezer shutting down")
			return
		default:
		}
		if backoff {
			
			if triggered != nil {
				triggered <- struct{}{}
				triggered = nil
			}
			select {
			case <-time.NewTimer(freezerRecheckInterval).C:
				backoff = false
			case triggered = <-f.trigger:
				backoff = false
			case <-f.quit:
				return
			}
		}
		
		hash := ReadHeadBlockHash(nfdb)
		if hash == (common.Hash{}) {
			log.Debug("Current full block hash unavailable") 
			backoff = true
			continue
		}
		number := ReadHeaderNumber(nfdb, hash)
		threshold := atomic.LoadUint64(&f.threshold)

		switch {
		case number == nil:
			log.Error("Current full block number unavailable", "hash", hash)
			backoff = true
			continue

		case *number < threshold:
			log.Debug("Current full block not old enough", "number", *number, "hash", hash, "delay", threshold)
			backoff = true
			continue

		case *number-threshold <= f.frozen:
			log.Debug("Ancient blocks frozen already", "number", *number, "hash", hash, "frozen", f.frozen)
			backoff = true
			continue
		}
		head := ReadHeader(nfdb, hash, *number)
		if head == nil {
			log.Error("Current full block unavailable", "number", *number, "hash", hash)
			backoff = true
			continue
		}
		
		limit := *number - threshold
		if limit-f.frozen > freezerBatchLimit {
			limit = f.frozen + freezerBatchLimit
		}
		var (
			start    = time.Now()
			first    = f.frozen
			ancients = make([]common.Hash, 0, limit-f.frozen)
		)
		for f.frozen <= limit {
			
			hash := ReadCanonicalHash(nfdb, f.frozen)
			if hash == (common.Hash{}) {
				log.Error("Canonical hash missing, can't freeze", "number", f.frozen)
				break
			}
			header := ReadHeaderRLP(nfdb, hash, f.frozen)
			if len(header) == 0 {
				log.Error("Block header missing, can't freeze", "number", f.frozen, "hash", hash)
				break
			}
			body := ReadBodyRLP(nfdb, hash, f.frozen)
			if len(body) == 0 {
				log.Error("Block body missing, can't freeze", "number", f.frozen, "hash", hash)
				break
			}
			receipts := ReadReceiptsRLP(nfdb, hash, f.frozen)
			if len(receipts) == 0 {
				log.Error("Block receipts missing, can't freeze", "number", f.frozen, "hash", hash)
				break
			}
			td := ReadTdRLP(nfdb, hash, f.frozen)
			if len(td) == 0 {
				log.Error("Total difficulty missing, can't freeze", "number", f.frozen, "hash", hash)
				break
			}
			log.Trace("Deep froze ancient block", "number", f.frozen, "hash", hash)
			
			if err := f.AppendAncient(f.frozen, hash[:], header, body, receipts, td); err != nil {
				break
			}
			ancients = append(ancients, hash)
		}
		
		if err := f.Sync(); err != nil {
			log.Crit("Failed to flush frozen tables", "err", err)
		}
		
		batch := db.NewBatch()
		for i := 0; i < len(ancients); i++ {
			
			if first+uint64(i) != 0 {
				DeleteBlockWithoutNumber(batch, ancients[i], first+uint64(i))
				DeleteCanonicalHash(batch, first+uint64(i))
			}
		}
		if err := batch.Write(); err != nil {
			log.Crit("Failed to delete frozen canonical blocks", "err", err)
		}
		batch.Reset()

		
		var dangling []common.Hash
		for number := first; number < f.frozen; number++ {
			
			if number != 0 {
				dangling = ReadAllHashes(db, number)
				for _, hash := range dangling {
					log.Trace("Deleting side chain", "number", number, "hash", hash)
					DeleteBlock(batch, hash, number)
				}
			}
		}
		if err := batch.Write(); err != nil {
			log.Crit("Failed to delete frozen side blocks", "err", err)
		}
		batch.Reset()

		
		if f.frozen > 0 {
			tip := f.frozen
			for len(dangling) > 0 {
				drop := make(map[common.Hash]struct{})
				for _, hash := range dangling {
					log.Debug("Dangling parent from freezer", "number", tip-1, "hash", hash)
					drop[hash] = struct{}{}
				}
				children := ReadAllHashes(db, tip)
				for i := 0; i < len(children); i++ {
					
					child := ReadHeader(nfdb, children[i], tip)
					if child == nil {
						log.Error("Missing dangling header", "number", tip, "hash", children[i])
						continue
					}
					if _, ok := drop[child.ParentHash]; !ok {
						children = append(children[:i], children[i+1:]...)
						i--
						continue
					}
					
					log.Debug("Deleting dangling block", "number", tip, "hash", children[i], "parent", child.ParentHash)
					DeleteBlock(batch, children[i], tip)
				}
				dangling = children
				tip++
			}
			if err := batch.Write(); err != nil {
				log.Crit("Failed to delete dangling side blocks", "err", err)
			}
		}
		
		context := []interface{}{
			"blocks", f.frozen - first, "elapsed", common.PrettyDuration(time.Since(start)), "number", f.frozen - 1,
		}
		if n := len(ancients); n > 0 {
			context = append(context, []interface{}{"hash", ancients[n-1]}...)
		}
		log.Info("Deep froze chain segment", context...)

		
		if f.frozen-first < freezerBatchLimit {
			backoff = true
		}
	}
}


func (f *freezer) repair() error {
	min := uint64(math.MaxUint64)
	for _, table := range f.tables {
		items := atomic.LoadUint64(&table.items)
		if min > items {
			min = items
		}
	}
	for _, table := range f.tables {
		if err := table.truncate(min); err != nil {
			return err
		}
	}
	atomic.StoreUint64(&f.frozen, min)
	return nil
}
