















package eth

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/bitutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
)

const (
	
	
	bloomServiceThreads = 16

	
	
	bloomFilterThreads = 3

	
	
	bloomRetrievalBatch = 16

	
	
	bloomRetrievalWait = time.Duration(0)
)



func (eth *Ethereum) startBloomHandlers(sectionSize uint64) {
	for i := 0; i < bloomServiceThreads; i++ {
		go func() {
			for {
				select {
				case <-eth.closeBloomHandler:
					return

				case request := <-eth.bloomRequests:
					task := <-request
					task.Bitsets = make([][]byte, len(task.Sections))
					for i, section := range task.Sections {
						head := rawdb.ReadCanonicalHash(eth.chainDb, (section+1)*sectionSize-1)
						if compVector, err := rawdb.ReadBloomBits(eth.chainDb, task.Bit, section, head); err == nil {
							if blob, err := bitutil.DecompressBytes(compVector, int(sectionSize/8)); err == nil {
								task.Bitsets[i] = blob
							} else {
								task.Error = err
							}
						} else {
							task.Error = err
						}
					}
					request <- task
				}
			}
		}()
	}
}

const (
	
	
	bloomThrottling = 100 * time.Millisecond
)



type BloomIndexer struct {
	size    uint64               
	db      ethdb.Database       
	gen     *bloombits.Generator 
	section uint64               
	head    common.Hash          
}



func NewBloomIndexer(db ethdb.Database, size, confirms uint64) *core.ChainIndexer {
	backend := &BloomIndexer{
		db:   db,
		size: size,
	}
	table := rawdb.NewTable(db, string(rawdb.BloomBitsIndexPrefix))

	return core.NewChainIndexer(db, table, backend, size, confirms, bloomThrottling, "bloombits")
}



func (b *BloomIndexer) Reset(ctx context.Context, section uint64, lastSectionHead common.Hash) error {
	gen, err := bloombits.NewGenerator(uint(b.size))
	b.gen, b.section, b.head = gen, section, common.Hash{}
	return err
}



func (b *BloomIndexer) Process(ctx context.Context, header *types.Header) error {
	b.gen.AddBloom(uint(header.Number.Uint64()-b.section*b.size), header.Bloom)
	b.head = header.Hash()
	return nil
}



func (b *BloomIndexer) Commit() error {
	batch := b.db.NewBatch()
	for i := 0; i < types.BloomBitLength; i++ {
		bits, err := b.gen.Bitset(uint(i))
		if err != nil {
			return err
		}
		rawdb.WriteBloomBits(batch, uint(i), b.section, b.head, bitutil.CompressBytes(bits))
	}
	return batch.Write()
}


func (b *BloomIndexer) Prune(threshold uint64) error {
	return nil
}
