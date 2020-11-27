
















package miner

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)


type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
}


type Config struct {
	Etherbase common.Address `toml:",omitempty"` 
	Notify    []string       `toml:",omitempty"` 
	ExtraData hexutil.Bytes  `toml:",omitempty"` 
	GasFloor  uint64         
	GasCeil   uint64         
	GasPrice  *big.Int       
	Recommit  time.Duration  
	Noverify  bool           
}


type Miner struct {
	mux      *event.TypeMux
	worker   *worker
	coinbase common.Address
	eth      Backend
	engine   consensus.Engine
	exitCh   chan struct{}
	startCh  chan common.Address
	stopCh   chan struct{}
}

func New(eth Backend, config *Config, chainConfig *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine, isLocalBlock func(block *types.Block) bool) *Miner {
	miner := &Miner{
		eth:     eth,
		mux:     mux,
		engine:  engine,
		exitCh:  make(chan struct{}),
		startCh: make(chan common.Address),
		stopCh:  make(chan struct{}),
		worker:  newWorker(config, chainConfig, engine, eth, mux, isLocalBlock, true),
	}
	go miner.update()

	return miner
}





func (miner *Miner) update() {
	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer func() {
		if !events.Closed() {
			events.Unsubscribe()
		}
	}()

	shouldStart := false
	canStart := true
	dlEventCh := events.Chan()
	for {
		select {
		case ev := <-dlEventCh:
			if ev == nil {
				
				dlEventCh = nil
				continue
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				wasMining := miner.Mining()
				miner.worker.stop()
				canStart = false
				if wasMining {
					
					shouldStart = true
					log.Info("Mining aborted due to sync")
				}
			case downloader.FailedEvent:
				canStart = true
				if shouldStart {
					miner.SetEtherbase(miner.coinbase)
					miner.worker.start()
				}
			case downloader.DoneEvent:
				canStart = true
				if shouldStart {
					miner.SetEtherbase(miner.coinbase)
					miner.worker.start()
				}
				
				events.Unsubscribe()
			}
		case addr := <-miner.startCh:
			miner.SetEtherbase(addr)
			if canStart {
				miner.worker.start()
			}
			shouldStart = true
		case <-miner.stopCh:
			shouldStart = false
			miner.worker.stop()
		case <-miner.exitCh:
			miner.worker.close()
			return
		}
	}
}

func (miner *Miner) Start(coinbase common.Address) {
	miner.startCh <- coinbase
}

func (miner *Miner) Stop() {
	miner.stopCh <- struct{}{}
}

func (miner *Miner) Close() {
	close(miner.exitCh)
}

func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}

func (miner *Miner) HashRate() uint64 {
	if pow, ok := miner.engine.(consensus.PoW); ok {
		return uint64(pow.Hashrate())
	}
	return 0
}

func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
}


func (miner *Miner) SetRecommitInterval(interval time.Duration) {
	miner.worker.setRecommitInterval(interval)
}


func (miner *Miner) Pending() (*types.Block, *state.StateDB) {
	return miner.worker.pending()
}






func (miner *Miner) PendingBlock() *types.Block {
	return miner.worker.pendingBlock()
}

func (miner *Miner) SetEtherbase(addr common.Address) {
	miner.coinbase = addr
	miner.worker.setEtherbase(addr)
}





func (miner *Miner) EnablePreseal() {
	miner.worker.enablePreseal()
}






func (miner *Miner) DisablePreseal() {
	miner.worker.disablePreseal()
}



func (miner *Miner) SubscribePendingLogs(ch chan<- []*types.Log) event.Subscription {
	return miner.worker.pendingLogsFeed.Subscribe(ch)
}
