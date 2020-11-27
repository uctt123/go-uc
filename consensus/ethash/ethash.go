
















package ethash

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	mmap "github.com/edsrzf/mmap-go"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hashicorp/golang-lru/simplelru"
)

var ErrInvalidDumpMagic = errors.New("invalid dump magic")

var (
	
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))

	
	sharedEthash = New(Config{"", 3, 0, false, "", 1, 0, false, ModeNormal, nil}, nil, false)

	
	algorithmRevision = 23

	
	dumpMagic = []uint32{0xbaddcafe, 0xfee1dead}
)



func isLittleEndian() bool {
	n := uint32(0x01020304)
	return *(*byte)(unsafe.Pointer(&n)) == 0x04
}


func memoryMap(path string, lock bool) (*os.File, mmap.MMap, []uint32, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, nil, nil, err
	}
	mem, buffer, err := memoryMapFile(file, false)
	if err != nil {
		file.Close()
		return nil, nil, nil, err
	}
	for i, magic := range dumpMagic {
		if buffer[i] != magic {
			mem.Unmap()
			file.Close()
			return nil, nil, nil, ErrInvalidDumpMagic
		}
	}
	if lock {
		if err := mem.Lock(); err != nil {
			mem.Unmap()
			file.Close()
			return nil, nil, nil, err
		}
	}
	return file, mem, buffer[len(dumpMagic):], err
}


func memoryMapFile(file *os.File, write bool) (mmap.MMap, []uint32, error) {
	
	flag := mmap.RDONLY
	if write {
		flag = mmap.RDWR
	}
	mem, err := mmap.Map(file, flag, 0)
	if err != nil {
		return nil, nil, err
	}
	
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&mem))
	header.Len /= 4
	header.Cap /= 4

	return mem, *(*[]uint32)(unsafe.Pointer(&header)), nil
}




func memoryMapAndGenerate(path string, size uint64, lock bool, generator func(buffer []uint32)) (*os.File, mmap.MMap, []uint32, error) {
	
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, nil, nil, err
	}
	
	temp := path + "." + strconv.Itoa(rand.Int())

	dump, err := os.Create(temp)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = dump.Truncate(int64(len(dumpMagic))*4 + int64(size)); err != nil {
		return nil, nil, nil, err
	}
	
	mem, buffer, err := memoryMapFile(dump, true)
	if err != nil {
		dump.Close()
		return nil, nil, nil, err
	}
	copy(buffer, dumpMagic)

	data := buffer[len(dumpMagic):]
	generator(data)

	if err := mem.Unmap(); err != nil {
		return nil, nil, nil, err
	}
	if err := dump.Close(); err != nil {
		return nil, nil, nil, err
	}
	if err := os.Rename(temp, path); err != nil {
		return nil, nil, nil, err
	}
	return memoryMap(path, lock)
}


type lru struct {
	what string
	new  func(epoch uint64) interface{}
	mu   sync.Mutex
	
	
	cache      *simplelru.LRU
	future     uint64
	futureItem interface{}
}



func newlru(what string, maxItems int, new func(epoch uint64) interface{}) *lru {
	if maxItems <= 0 {
		maxItems = 1
	}
	cache, _ := simplelru.NewLRU(maxItems, func(key, value interface{}) {
		log.Trace("Evicted ethash "+what, "epoch", key)
	})
	return &lru{what: what, new: new, cache: cache}
}




func (lru *lru) get(epoch uint64) (item, future interface{}) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	
	item, ok := lru.cache.Get(epoch)
	if !ok {
		if lru.future > 0 && lru.future == epoch {
			item = lru.futureItem
		} else {
			log.Trace("Requiring new ethash "+lru.what, "epoch", epoch)
			item = lru.new(epoch)
		}
		lru.cache.Add(epoch, item)
	}
	
	if epoch < maxEpoch-1 && lru.future < epoch+1 {
		log.Trace("Requiring new future ethash "+lru.what, "epoch", epoch+1)
		future = lru.new(epoch + 1)
		lru.future = epoch + 1
		lru.futureItem = future
	}
	return item, future
}


type cache struct {
	epoch uint64    
	dump  *os.File  
	mmap  mmap.MMap 
	cache []uint32  
	once  sync.Once 
}



func newCache(epoch uint64) interface{} {
	return &cache{epoch: epoch}
}


func (c *cache) generate(dir string, limit int, lock bool, test bool) {
	c.once.Do(func() {
		size := cacheSize(c.epoch*epochLength + 1)
		seed := seedHash(c.epoch*epochLength + 1)
		if test {
			size = 1024
		}
		
		if dir == "" {
			c.cache = make([]uint32, size/4)
			generateCache(c.cache, c.epoch, seed)
			return
		}
		
		var endian string
		if !isLittleEndian() {
			endian = ".be"
		}
		path := filepath.Join(dir, fmt.Sprintf("cache-R%d-%x%s", algorithmRevision, seed[:8], endian))
		logger := log.New("epoch", c.epoch)

		
		
		runtime.SetFinalizer(c, (*cache).finalizer)

		
		var err error
		c.dump, c.mmap, c.cache, err = memoryMap(path, lock)
		if err == nil {
			logger.Debug("Loaded old ethash cache from disk")
			return
		}
		logger.Debug("Failed to load old ethash cache", "err", err)

		
		c.dump, c.mmap, c.cache, err = memoryMapAndGenerate(path, size, lock, func(buffer []uint32) { generateCache(buffer, c.epoch, seed) })
		if err != nil {
			logger.Error("Failed to generate mapped ethash cache", "err", err)

			c.cache = make([]uint32, size/4)
			generateCache(c.cache, c.epoch, seed)
		}
		
		for ep := int(c.epoch) - limit; ep >= 0; ep-- {
			seed := seedHash(uint64(ep)*epochLength + 1)
			path := filepath.Join(dir, fmt.Sprintf("cache-R%d-%x%s", algorithmRevision, seed[:8], endian))
			os.Remove(path)
		}
	})
}


func (c *cache) finalizer() {
	if c.mmap != nil {
		c.mmap.Unmap()
		c.dump.Close()
		c.mmap, c.dump = nil, nil
	}
}


type dataset struct {
	epoch   uint64    
	dump    *os.File  
	mmap    mmap.MMap 
	dataset []uint32  
	once    sync.Once 
	done    uint32    
}



func newDataset(epoch uint64) interface{} {
	return &dataset{epoch: epoch}
}


func (d *dataset) generate(dir string, limit int, lock bool, test bool) {
	d.once.Do(func() {
		
		defer atomic.StoreUint32(&d.done, 1)

		csize := cacheSize(d.epoch*epochLength + 1)
		dsize := datasetSize(d.epoch*epochLength + 1)
		seed := seedHash(d.epoch*epochLength + 1)
		if test {
			csize = 1024
			dsize = 32 * 1024
		}
		
		if dir == "" {
			cache := make([]uint32, csize/4)
			generateCache(cache, d.epoch, seed)

			d.dataset = make([]uint32, dsize/4)
			generateDataset(d.dataset, d.epoch, cache)

			return
		}
		
		var endian string
		if !isLittleEndian() {
			endian = ".be"
		}
		path := filepath.Join(dir, fmt.Sprintf("full-R%d-%x%s", algorithmRevision, seed[:8], endian))
		logger := log.New("epoch", d.epoch)

		
		
		runtime.SetFinalizer(d, (*dataset).finalizer)

		
		var err error
		d.dump, d.mmap, d.dataset, err = memoryMap(path, lock)
		if err == nil {
			logger.Debug("Loaded old ethash dataset from disk")
			return
		}
		logger.Debug("Failed to load old ethash dataset", "err", err)

		
		cache := make([]uint32, csize/4)
		generateCache(cache, d.epoch, seed)

		d.dump, d.mmap, d.dataset, err = memoryMapAndGenerate(path, dsize, lock, func(buffer []uint32) { generateDataset(buffer, d.epoch, cache) })
		if err != nil {
			logger.Error("Failed to generate mapped ethash dataset", "err", err)

			d.dataset = make([]uint32, dsize/2)
			generateDataset(d.dataset, d.epoch, cache)
		}
		
		for ep := int(d.epoch) - limit; ep >= 0; ep-- {
			seed := seedHash(uint64(ep)*epochLength + 1)
			path := filepath.Join(dir, fmt.Sprintf("full-R%d-%x%s", algorithmRevision, seed[:8], endian))
			os.Remove(path)
		}
	})
}




func (d *dataset) generated() bool {
	return atomic.LoadUint32(&d.done) == 1
}


func (d *dataset) finalizer() {
	if d.mmap != nil {
		d.mmap.Unmap()
		d.dump.Close()
		d.mmap, d.dump = nil, nil
	}
}


func MakeCache(block uint64, dir string) {
	c := cache{epoch: block / epochLength}
	c.generate(dir, math.MaxInt32, false, false)
}


func MakeDataset(block uint64, dir string) {
	d := dataset{epoch: block / epochLength}
	d.generate(dir, math.MaxInt32, false, false)
}


type Mode uint

const (
	ModeNormal Mode = iota
	ModeShared
	ModeTest
	ModeFake
	ModeFullFake
)


type Config struct {
	CacheDir         string
	CachesInMem      int
	CachesOnDisk     int
	CachesLockMmap   bool
	DatasetDir       string
	DatasetsInMem    int
	DatasetsOnDisk   int
	DatasetsLockMmap bool
	PowMode          Mode

	Log log.Logger `toml:"-"`
}



type Ethash struct {
	config Config

	caches   *lru 
	datasets *lru 

	
	rand     *rand.Rand    
	threads  int           
	update   chan struct{} 
	hashrate metrics.Meter 
	remote   *remoteSealer

	
	shared    *Ethash       
	fakeFail  uint64        
	fakeDelay time.Duration 

	lock      sync.Mutex 
	closeOnce sync.Once  
}




func New(config Config, notify []string, noverify bool) *Ethash {
	if config.Log == nil {
		config.Log = log.Root()
	}
	if config.CachesInMem <= 0 {
		config.Log.Warn("One ethash cache must always be in memory", "requested", config.CachesInMem)
		config.CachesInMem = 1
	}
	if config.CacheDir != "" && config.CachesOnDisk > 0 {
		config.Log.Info("Disk storage enabled for ethash caches", "dir", config.CacheDir, "count", config.CachesOnDisk)
	}
	if config.DatasetDir != "" && config.DatasetsOnDisk > 0 {
		config.Log.Info("Disk storage enabled for ethash DAGs", "dir", config.DatasetDir, "count", config.DatasetsOnDisk)
	}
	ethash := &Ethash{
		config:   config,
		caches:   newlru("cache", config.CachesInMem, newCache),
		datasets: newlru("dataset", config.DatasetsInMem, newDataset),
		update:   make(chan struct{}),
		hashrate: metrics.NewMeterForced(),
	}
	ethash.remote = startRemoteSealer(ethash, notify, noverify)
	return ethash
}



func NewTester(notify []string, noverify bool) *Ethash {
	ethash := &Ethash{
		config:   Config{PowMode: ModeTest, Log: log.Root()},
		caches:   newlru("cache", 1, newCache),
		datasets: newlru("dataset", 1, newDataset),
		update:   make(chan struct{}),
		hashrate: metrics.NewMeterForced(),
	}
	ethash.remote = startRemoteSealer(ethash, notify, noverify)
	return ethash
}




func NewFaker() *Ethash {
	return &Ethash{
		config: Config{
			PowMode: ModeFake,
			Log:     log.Root(),
		},
	}
}




func NewFakeFailer(fail uint64) *Ethash {
	return &Ethash{
		config: Config{
			PowMode: ModeFake,
			Log:     log.Root(),
		},
		fakeFail: fail,
	}
}




func NewFakeDelayer(delay time.Duration) *Ethash {
	return &Ethash{
		config: Config{
			PowMode: ModeFake,
			Log:     log.Root(),
		},
		fakeDelay: delay,
	}
}



func NewFullFaker() *Ethash {
	return &Ethash{
		config: Config{
			PowMode: ModeFullFake,
			Log:     log.Root(),
		},
	}
}



func NewShared() *Ethash {
	return &Ethash{shared: sharedEthash}
}


func (ethash *Ethash) Close() error {
	var err error
	ethash.closeOnce.Do(func() {
		
		if ethash.remote == nil {
			return
		}
		close(ethash.remote.requestExit)
		<-ethash.remote.exitCh
	})
	return err
}




func (ethash *Ethash) cache(block uint64) *cache {
	epoch := block / epochLength
	currentI, futureI := ethash.caches.get(epoch)
	current := currentI.(*cache)

	
	current.generate(ethash.config.CacheDir, ethash.config.CachesOnDisk, ethash.config.CachesLockMmap, ethash.config.PowMode == ModeTest)

	
	if futureI != nil {
		future := futureI.(*cache)
		go future.generate(ethash.config.CacheDir, ethash.config.CachesOnDisk, ethash.config.CachesLockMmap, ethash.config.PowMode == ModeTest)
	}
	return current
}







func (ethash *Ethash) dataset(block uint64, async bool) *dataset {
	
	epoch := block / epochLength
	currentI, futureI := ethash.datasets.get(epoch)
	current := currentI.(*dataset)

	
	if async && !current.generated() {
		go func() {
			current.generate(ethash.config.DatasetDir, ethash.config.DatasetsOnDisk, ethash.config.DatasetsLockMmap, ethash.config.PowMode == ModeTest)

			if futureI != nil {
				future := futureI.(*dataset)
				future.generate(ethash.config.DatasetDir, ethash.config.DatasetsOnDisk, ethash.config.DatasetsLockMmap, ethash.config.PowMode == ModeTest)
			}
		}()
	} else {
		
		current.generate(ethash.config.DatasetDir, ethash.config.DatasetsOnDisk, ethash.config.DatasetsLockMmap, ethash.config.PowMode == ModeTest)

		if futureI != nil {
			future := futureI.(*dataset)
			go future.generate(ethash.config.DatasetDir, ethash.config.DatasetsOnDisk, ethash.config.DatasetsLockMmap, ethash.config.PowMode == ModeTest)
		}
	}
	return current
}



func (ethash *Ethash) Threads() int {
	ethash.lock.Lock()
	defer ethash.lock.Unlock()

	return ethash.threads
}






func (ethash *Ethash) SetThreads(threads int) {
	ethash.lock.Lock()
	defer ethash.lock.Unlock()

	
	if ethash.shared != nil {
		ethash.shared.SetThreads(threads)
		return
	}
	
	ethash.threads = threads
	select {
	case ethash.update <- struct{}{}:
	default:
	}
}





func (ethash *Ethash) Hashrate() float64 {
	
	if ethash.config.PowMode != ModeNormal && ethash.config.PowMode != ModeTest {
		return ethash.hashrate.Rate1()
	}
	var res = make(chan uint64, 1)

	select {
	case ethash.remote.fetchRateCh <- res:
	case <-ethash.remote.exitCh:
		
		return ethash.hashrate.Rate1()
	}

	
	return ethash.hashrate.Rate1() + float64(<-res)
}


func (ethash *Ethash) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	
	
	return []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   &API{ethash},
			Public:    true,
		},
		{
			Namespace: "ethash",
			Version:   "1.0",
			Service:   &API{ethash},
			Public:    true,
		},
	}
}



func SeedHash(block uint64) []byte {
	return seedHash(block)
}
