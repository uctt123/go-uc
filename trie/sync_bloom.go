















package trie

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/steakknife/bloomfilter"
)

var (
	bloomAddMeter   = metrics.NewRegisteredMeter("trie/bloom/add", nil)
	bloomLoadMeter  = metrics.NewRegisteredMeter("trie/bloom/load", nil)
	bloomTestMeter  = metrics.NewRegisteredMeter("trie/bloom/test", nil)
	bloomMissMeter  = metrics.NewRegisteredMeter("trie/bloom/miss", nil)
	bloomFaultMeter = metrics.NewRegisteredMeter("trie/bloom/fault", nil)
	bloomErrorGauge = metrics.NewRegisteredGauge("trie/bloom/error", nil)
)




type syncBloomHasher []byte

func (f syncBloomHasher) Write(p []byte) (n int, err error) { panic("not implemented") }
func (f syncBloomHasher) Sum(b []byte) []byte               { panic("not implemented") }
func (f syncBloomHasher) Reset()                            { panic("not implemented") }
func (f syncBloomHasher) BlockSize() int                    { panic("not implemented") }
func (f syncBloomHasher) Size() int                         { return 8 }
func (f syncBloomHasher) Sum64() uint64                     { return binary.BigEndian.Uint64(f) }





type SyncBloom struct {
	bloom  *bloomfilter.Filter
	inited uint32
	closer sync.Once
	closed uint32
	pend   sync.WaitGroup
}



func NewSyncBloom(memory uint64, database ethdb.Iteratee) *SyncBloom {
	
	bloom, err := bloomfilter.New(memory*1024*1024*8, 3)
	if err != nil {
		panic(fmt.Sprintf("failed to create bloom: %v", err))
	}
	log.Info("Allocated fast sync bloom", "size", common.StorageSize(memory*1024*1024))

	
	b := &SyncBloom{
		bloom: bloom,
	}
	b.pend.Add(2)
	go func() {
		defer b.pend.Done()
		b.init(database)
	}()
	go func() {
		defer b.pend.Done()
		b.meter()
	}()
	return b
}


func (b *SyncBloom) init(database ethdb.Iteratee) {
	
	
	
	
	
	
	
	it := database.NewIterator(nil, nil)

	var (
		start = time.Now()
		swap  = time.Now()
	)
	for it.Next() && atomic.LoadUint32(&b.closed) == 0 {
		
		key := it.Key()
		if len(key) == common.HashLength {
			b.bloom.Add(syncBloomHasher(key))
			bloomLoadMeter.Mark(1)
		}
		
		if ok, hash := rawdb.IsCodeKey(key); ok {
			b.bloom.Add(syncBloomHasher(hash))
			bloomLoadMeter.Mark(1)
		}
		
		if time.Since(swap) > 8*time.Second {
			key := common.CopyBytes(it.Key())

			it.Release()
			it = database.NewIterator(nil, key)

			log.Info("Initializing fast sync bloom", "items", b.bloom.N(), "errorrate", b.errorRate(), "elapsed", common.PrettyDuration(time.Since(start)))
			swap = time.Now()
		}
	}
	it.Release()

	
	log.Info("Initialized fast sync bloom", "items", b.bloom.N(), "errorrate", b.errorRate(), "elapsed", common.PrettyDuration(time.Since(start)))
	atomic.StoreUint32(&b.inited, 1)
}



func (b *SyncBloom) meter() {
	for {
		
		bloomErrorGauge.Update(int64(b.errorRate() * 100000))

		
		for i := 0; i < 10; i++ {
			if atomic.LoadUint32(&b.closed) == 1 {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}



func (b *SyncBloom) Close() error {
	b.closer.Do(func() {
		
		atomic.StoreUint32(&b.closed, 1)
		b.pend.Wait()

		
		log.Info("Deallocated fast sync bloom", "items", b.bloom.N(), "errorrate", b.errorRate())

		atomic.StoreUint32(&b.inited, 0)
		b.bloom = nil
	})
	return nil
}


func (b *SyncBloom) Add(hash []byte) {
	if atomic.LoadUint32(&b.closed) == 1 {
		return
	}
	b.bloom.Add(syncBloomHasher(hash))
	bloomAddMeter.Mark(1)
}






func (b *SyncBloom) Contains(hash []byte) bool {
	bloomTestMeter.Mark(1)
	if atomic.LoadUint32(&b.inited) == 0 {
		
		
		
		return true
	}
	
	maybe := b.bloom.Contains(syncBloomHasher(hash))
	if !maybe {
		bloomMissMeter.Mark(1)
	}
	return maybe
}






func (b *SyncBloom) errorRate() float64 {
	k := float64(b.bloom.K())
	n := float64(b.bloom.N())
	m := float64(b.bloom.M())

	return math.Pow(1.0-math.Exp((-k)*(n+0.5)/(m-1)), k)
}
