

















package light

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
)

var (
	bodyCacheLimit  = 256
	blockCacheLimit = 256
)




type LightChain struct {
	hc            *core.HeaderChain
	indexerConfig *IndexerConfig
	chainDb       ethdb.Database
	engine        consensus.Engine
	odr           OdrBackend
	chainFeed     event.Feed
	chainSideFeed event.Feed
	chainHeadFeed event.Feed
	scope         event.SubscriptionScope
	genesisBlock  *types.Block

	bodyCache    *lru.Cache 
	bodyRLPCache *lru.Cache 
	blockCache   *lru.Cache 

	chainmu sync.RWMutex 
	quit    chan struct{}
	wg      sync.WaitGroup

	
	running          int32 
	procInterrupt    int32 
	disableCheckFreq int32 
}




func NewLightChain(odr OdrBackend, config *params.ChainConfig, engine consensus.Engine, checkpoint *params.TrustedCheckpoint) (*LightChain, error) {
	bodyCache, _ := lru.New(bodyCacheLimit)
	bodyRLPCache, _ := lru.New(bodyCacheLimit)
	blockCache, _ := lru.New(blockCacheLimit)

	bc := &LightChain{
		chainDb:       odr.Database(),
		indexerConfig: odr.IndexerConfig(),
		odr:           odr,
		quit:          make(chan struct{}),
		bodyCache:     bodyCache,
		bodyRLPCache:  bodyRLPCache,
		blockCache:    blockCache,
		engine:        engine,
	}
	var err error
	bc.hc, err = core.NewHeaderChain(odr.Database(), config, bc.engine, bc.getProcInterrupt)
	if err != nil {
		return nil, err
	}
	bc.genesisBlock, _ = bc.GetBlockByNumber(NoOdr, 0)
	if bc.genesisBlock == nil {
		return nil, core.ErrNoGenesis
	}
	if checkpoint != nil {
		bc.AddTrustedCheckpoint(checkpoint)
	}
	if err := bc.loadLastState(); err != nil {
		return nil, err
	}
	
	for hash := range core.BadHashes {
		if header := bc.GetHeaderByHash(hash); header != nil {
			log.Error("Found bad hash, rewinding chain", "number", header.Number, "hash", header.ParentHash)
			bc.SetHead(header.Number.Uint64() - 1)
			log.Info("Chain rewind was successful, resuming normal operation")
		}
	}
	return bc, nil
}


func (lc *LightChain) AddTrustedCheckpoint(cp *params.TrustedCheckpoint) {
	if lc.odr.ChtIndexer() != nil {
		StoreChtRoot(lc.chainDb, cp.SectionIndex, cp.SectionHead, cp.CHTRoot)
		lc.odr.ChtIndexer().AddCheckpoint(cp.SectionIndex, cp.SectionHead)
	}
	if lc.odr.BloomTrieIndexer() != nil {
		StoreBloomTrieRoot(lc.chainDb, cp.SectionIndex, cp.SectionHead, cp.BloomRoot)
		lc.odr.BloomTrieIndexer().AddCheckpoint(cp.SectionIndex, cp.SectionHead)
	}
	if lc.odr.BloomIndexer() != nil {
		lc.odr.BloomIndexer().AddCheckpoint(cp.SectionIndex, cp.SectionHead)
	}
	log.Info("Added trusted checkpoint", "block", (cp.SectionIndex+1)*lc.indexerConfig.ChtSize-1, "hash", cp.SectionHead)
}

func (lc *LightChain) getProcInterrupt() bool {
	return atomic.LoadInt32(&lc.procInterrupt) == 1
}


func (lc *LightChain) Odr() OdrBackend {
	return lc.odr
}


func (lc *LightChain) HeaderChain() *core.HeaderChain {
	return lc.hc
}



func (lc *LightChain) loadLastState() error {
	if head := rawdb.ReadHeadHeaderHash(lc.chainDb); head == (common.Hash{}) {
		
		lc.Reset()
	} else {
		header := lc.GetHeaderByHash(head)
		if header == nil {
			
			lc.Reset()
		} else {
			lc.hc.SetCurrentHeader(header)
		}
	}
	
	header := lc.hc.CurrentHeader()
	headerTd := lc.GetTd(header.Hash(), header.Number.Uint64())
	log.Info("Loaded most recent local header", "number", header.Number, "hash", header.Hash(), "td", headerTd, "age", common.PrettyAge(time.Unix(int64(header.Time), 0)))
	return nil
}



func (lc *LightChain) SetHead(head uint64) error {
	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	lc.hc.SetHead(head, nil, nil)
	return lc.loadLastState()
}


func (lc *LightChain) GasLimit() uint64 {
	return lc.hc.CurrentHeader().GasLimit
}


func (lc *LightChain) Reset() {
	lc.ResetWithGenesisBlock(lc.genesisBlock)
}



func (lc *LightChain) ResetWithGenesisBlock(genesis *types.Block) {
	
	lc.SetHead(0)

	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	
	batch := lc.chainDb.NewBatch()
	rawdb.WriteTd(batch, genesis.Hash(), genesis.NumberU64(), genesis.Difficulty())
	rawdb.WriteBlock(batch, genesis)
	rawdb.WriteHeadHeaderHash(batch, genesis.Hash())
	if err := batch.Write(); err != nil {
		log.Crit("Failed to reset genesis block", "err", err)
	}
	lc.genesisBlock = genesis
	lc.hc.SetGenesis(lc.genesisBlock.Header())
	lc.hc.SetCurrentHeader(lc.genesisBlock.Header())
}




func (lc *LightChain) Engine() consensus.Engine { return lc.engine }


func (lc *LightChain) Genesis() *types.Block {
	return lc.genesisBlock
}

func (lc *LightChain) StateCache() state.Database {
	panic("not implemented")
}



func (lc *LightChain) GetBody(ctx context.Context, hash common.Hash) (*types.Body, error) {
	
	if cached, ok := lc.bodyCache.Get(hash); ok {
		body := cached.(*types.Body)
		return body, nil
	}
	number := lc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil, errors.New("unknown block")
	}
	body, err := GetBody(ctx, lc.odr, hash, *number)
	if err != nil {
		return nil, err
	}
	
	lc.bodyCache.Add(hash, body)
	return body, nil
}



func (lc *LightChain) GetBodyRLP(ctx context.Context, hash common.Hash) (rlp.RawValue, error) {
	
	if cached, ok := lc.bodyRLPCache.Get(hash); ok {
		return cached.(rlp.RawValue), nil
	}
	number := lc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil, errors.New("unknown block")
	}
	body, err := GetBodyRLP(ctx, lc.odr, hash, *number)
	if err != nil {
		return nil, err
	}
	
	lc.bodyRLPCache.Add(hash, body)
	return body, nil
}



func (lc *LightChain) HasBlock(hash common.Hash, number uint64) bool {
	blk, _ := lc.GetBlock(NoOdr, hash, number)
	return blk != nil
}



func (lc *LightChain) GetBlock(ctx context.Context, hash common.Hash, number uint64) (*types.Block, error) {
	
	if block, ok := lc.blockCache.Get(hash); ok {
		return block.(*types.Block), nil
	}
	block, err := GetBlock(ctx, lc.odr, hash, number)
	if err != nil {
		return nil, err
	}
	
	lc.blockCache.Add(block.Hash(), block)
	return block, nil
}



func (lc *LightChain) GetBlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	number := lc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil, errors.New("unknown block")
	}
	return lc.GetBlock(ctx, hash, *number)
}



func (lc *LightChain) GetBlockByNumber(ctx context.Context, number uint64) (*types.Block, error) {
	hash, err := GetCanonicalHash(ctx, lc.odr, number)
	if hash == (common.Hash{}) || err != nil {
		return nil, err
	}
	return lc.GetBlock(ctx, hash, number)
}



func (lc *LightChain) Stop() {
	if !atomic.CompareAndSwapInt32(&lc.running, 0, 1) {
		return
	}
	close(lc.quit)
	lc.StopInsert()
	lc.wg.Wait()
	log.Info("Blockchain stopped")
}




func (lc *LightChain) StopInsert() {
	atomic.StoreInt32(&lc.procInterrupt, 1)
}



func (lc *LightChain) Rollback(chain []common.Hash) {
	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	batch := lc.chainDb.NewBatch()
	for i := len(chain) - 1; i >= 0; i-- {
		hash := chain[i]

		
		
		
		
		if head := lc.hc.CurrentHeader(); head.Hash() == hash {
			rawdb.WriteHeadHeaderHash(batch, head.ParentHash)
			lc.hc.SetCurrentHeader(lc.GetHeader(head.ParentHash, head.Number.Uint64()-1))
		}
	}
	if err := batch.Write(); err != nil {
		log.Crit("Failed to rollback light chain", "error", err)
	}
}



func (lc *LightChain) postChainEvents(events []interface{}) {
	for _, event := range events {
		switch ev := event.(type) {
		case core.ChainEvent:
			if lc.CurrentHeader().Hash() == ev.Hash {
				lc.chainHeadFeed.Send(core.ChainHeadEvent{Block: ev.Block})
			}
			lc.chainFeed.Send(ev)
		case core.ChainSideEvent:
			lc.chainSideFeed.Send(ev)
		}
	}
}












func (lc *LightChain) InsertHeaderChain(chain []*types.Header, checkFreq int) (int, error) {
	if atomic.LoadInt32(&lc.disableCheckFreq) == 1 {
		checkFreq = 0
	}
	start := time.Now()
	if i, err := lc.hc.ValidateHeaderChain(chain, checkFreq); err != nil {
		return i, err
	}

	
	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	lc.wg.Add(1)
	defer lc.wg.Done()

	var events []interface{}
	whFunc := func(header *types.Header) error {
		status, err := lc.hc.WriteHeader(header)

		switch status {
		case core.CanonStatTy:
			log.Debug("Inserted new header", "number", header.Number, "hash", header.Hash())
			events = append(events, core.ChainEvent{Block: types.NewBlockWithHeader(header), Hash: header.Hash()})

		case core.SideStatTy:
			log.Debug("Inserted forked header", "number", header.Number, "hash", header.Hash())
			events = append(events, core.ChainSideEvent{Block: types.NewBlockWithHeader(header)})
		}
		return err
	}
	i, err := lc.hc.InsertHeaderChain(chain, whFunc, start)
	lc.postChainEvents(events)
	return i, err
}



func (lc *LightChain) CurrentHeader() *types.Header {
	return lc.hc.CurrentHeader()
}



func (lc *LightChain) GetTd(hash common.Hash, number uint64) *big.Int {
	return lc.hc.GetTd(hash, number)
}



func (lc *LightChain) GetTdByHash(hash common.Hash) *big.Int {
	return lc.hc.GetTdByHash(hash)
}



func (lc *LightChain) GetTdOdr(ctx context.Context, hash common.Hash, number uint64) *big.Int {
	td := lc.GetTd(hash, number)
	if td != nil {
		return td
	}
	td, _ = GetTd(ctx, lc.odr, hash, number)
	return td
}



func (lc *LightChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	return lc.hc.GetHeader(hash, number)
}



func (lc *LightChain) GetHeaderByHash(hash common.Hash) *types.Header {
	return lc.hc.GetHeaderByHash(hash)
}



func (lc *LightChain) HasHeader(hash common.Hash, number uint64) bool {
	return lc.hc.HasHeader(hash, number)
}


func (bc *LightChain) GetCanonicalHash(number uint64) common.Hash {
	return bc.hc.GetCanonicalHash(number)
}



func (lc *LightChain) GetBlockHashesFromHash(hash common.Hash, max uint64) []common.Hash {
	return lc.hc.GetBlockHashesFromHash(hash, max)
}






func (lc *LightChain) GetAncestor(hash common.Hash, number, ancestor uint64, maxNonCanonical *uint64) (common.Hash, uint64) {
	return lc.hc.GetAncestor(hash, number, ancestor, maxNonCanonical)
}



func (lc *LightChain) GetHeaderByNumber(number uint64) *types.Header {
	return lc.hc.GetHeaderByNumber(number)
}



func (lc *LightChain) GetHeaderByNumberOdr(ctx context.Context, number uint64) (*types.Header, error) {
	if header := lc.hc.GetHeaderByNumber(number); header != nil {
		return header, nil
	}
	return GetHeaderByNumber(ctx, lc.odr, number)
}


func (lc *LightChain) Config() *params.ChainConfig { return lc.hc.Config() }






func (lc *LightChain) SyncCheckpoint(ctx context.Context, checkpoint *params.TrustedCheckpoint) bool {
	
	head := lc.CurrentHeader().Number.Uint64()

	latest := (checkpoint.SectionIndex+1)*lc.indexerConfig.ChtSize - 1
	if clique := lc.hc.Config().Clique; clique != nil {
		latest -= latest % clique.Epoch 
	}
	if head >= latest {
		return true
	}
	
	if header, err := GetHeaderByNumber(ctx, lc.odr, latest); header != nil && err == nil {
		lc.chainmu.Lock()
		defer lc.chainmu.Unlock()

		
		if lc.hc.CurrentHeader().Number.Uint64() < header.Number.Uint64() {
			log.Info("Updated latest header based on CHT", "number", header.Number, "hash", header.Hash(), "age", common.PrettyAge(time.Unix(int64(header.Time), 0)))
			rawdb.WriteHeadHeaderHash(lc.chainDb, header.Hash())
			lc.hc.SetCurrentHeader(header)
		}
		return true
	}
	return false
}



func (lc *LightChain) LockChain() {
	lc.chainmu.RLock()
}


func (lc *LightChain) UnlockChain() {
	lc.chainmu.RUnlock()
}


func (lc *LightChain) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return lc.scope.Track(lc.chainFeed.Subscribe(ch))
}


func (lc *LightChain) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return lc.scope.Track(lc.chainHeadFeed.Subscribe(ch))
}


func (lc *LightChain) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return lc.scope.Track(lc.chainSideFeed.Subscribe(ch))
}



func (lc *LightChain) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return lc.scope.Track(new(event.Feed).Subscribe(ch))
}



func (lc *LightChain) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return lc.scope.Track(new(event.Feed).Subscribe(ch))
}


func (lc *LightChain) DisableCheckFreq() {
	atomic.StoreInt32(&lc.disableCheckFreq, 1)
}


func (lc *LightChain) EnableCheckFreq() {
	atomic.StoreInt32(&lc.disableCheckFreq, 0)
}
