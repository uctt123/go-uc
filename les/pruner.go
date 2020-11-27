















package les

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
)


type pruner struct {
	db       ethdb.Database
	indexers []*core.ChainIndexer
	closeCh  chan struct{}
	wg       sync.WaitGroup
}


func newPruner(db ethdb.Database, indexers ...*core.ChainIndexer) *pruner {
	pruner := &pruner{
		db:       db,
		indexers: indexers,
		closeCh:  make(chan struct{}),
	}
	pruner.wg.Add(1)
	go pruner.loop()
	return pruner
}


func (p *pruner) close() {
	close(p.closeCh)
	p.wg.Wait()
}






func (p *pruner) loop() {
	defer p.wg.Done()

	
	var cleanTicker = time.NewTicker(12 * time.Hour)

	
	
	
	
	pruning := func() {
		min := uint64(math.MaxUint64)
		for _, indexer := range p.indexers {
			sections, _, _ := indexer.Sections()
			if sections < min {
				min = sections
			}
		}
		
		if min < 2 || len(p.indexers) == 0 {
			return
		}
		for _, indexer := range p.indexers {
			if err := indexer.Prune(min - 2); err != nil {
				log.Debug("Failed to prune historical data", "err", err)
				return
			}
		}
		p.db.Compact(nil, nil) 
	}
	for {
		pruning()
		select {
		case <-cleanTicker.C:
		case <-p.closeCh:
			return
		}
	}
}
