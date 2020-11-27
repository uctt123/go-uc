

















package filters

import (
	"context"
	"fmt"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)



type Type byte

const (
	
	UnknownSubscription Type = iota
	
	LogsSubscription
	
	PendingLogsSubscription
	
	MinedAndPendingLogsSubscription
	
	
	PendingTransactionsSubscription
	
	BlocksSubscription
	
	LastIndexSubscription
)

const (
	
	
	txChanSize = 4096
	
	rmLogsChanSize = 10
	
	logsChanSize = 10
	
	chainEvChanSize = 10
)

type subscription struct {
	id        rpc.ID
	typ       Type
	created   time.Time
	logsCrit  ethereum.FilterQuery
	logs      chan []*types.Log
	hashes    chan []common.Hash
	headers   chan *types.Header
	installed chan struct{} 
	err       chan error    
}



type EventSystem struct {
	backend   Backend
	lightMode bool
	lastHead  *types.Header

	
	txsSub         event.Subscription 
	logsSub        event.Subscription 
	rmLogsSub      event.Subscription 
	pendingLogsSub event.Subscription 
	chainSub       event.Subscription 

	
	install       chan *subscription         
	uninstall     chan *subscription         
	txsCh         chan core.NewTxsEvent      
	logsCh        chan []*types.Log          
	pendingLogsCh chan []*types.Log          
	rmLogsCh      chan core.RemovedLogsEvent 
	chainCh       chan core.ChainEvent       
}







func NewEventSystem(backend Backend, lightMode bool) *EventSystem {
	m := &EventSystem{
		backend:       backend,
		lightMode:     lightMode,
		install:       make(chan *subscription),
		uninstall:     make(chan *subscription),
		txsCh:         make(chan core.NewTxsEvent, txChanSize),
		logsCh:        make(chan []*types.Log, logsChanSize),
		rmLogsCh:      make(chan core.RemovedLogsEvent, rmLogsChanSize),
		pendingLogsCh: make(chan []*types.Log, logsChanSize),
		chainCh:       make(chan core.ChainEvent, chainEvChanSize),
	}

	
	m.txsSub = m.backend.SubscribeNewTxsEvent(m.txsCh)
	m.logsSub = m.backend.SubscribeLogsEvent(m.logsCh)
	m.rmLogsSub = m.backend.SubscribeRemovedLogsEvent(m.rmLogsCh)
	m.chainSub = m.backend.SubscribeChainEvent(m.chainCh)
	m.pendingLogsSub = m.backend.SubscribePendingLogsEvent(m.pendingLogsCh)

	
	if m.txsSub == nil || m.logsSub == nil || m.rmLogsSub == nil || m.chainSub == nil || m.pendingLogsSub == nil {
		log.Crit("Subscribe for event system failed")
	}

	go m.eventLoop()
	return m
}


type Subscription struct {
	ID        rpc.ID
	f         *subscription
	es        *EventSystem
	unsubOnce sync.Once
}


func (sub *Subscription) Err() <-chan error {
	return sub.f.err
}


func (sub *Subscription) Unsubscribe() {
	sub.unsubOnce.Do(func() {
	uninstallLoop:
		for {
			
			
			
			
			select {
			case sub.es.uninstall <- sub.f:
				break uninstallLoop
			case <-sub.f.logs:
			case <-sub.f.hashes:
			case <-sub.f.headers:
			}
		}

		
		
		
		<-sub.Err()
	})
}


func (es *EventSystem) subscribe(sub *subscription) *Subscription {
	es.install <- sub
	<-sub.installed
	return &Subscription{ID: sub.id, f: sub, es: es}
}




func (es *EventSystem) SubscribeLogs(crit ethereum.FilterQuery, logs chan []*types.Log) (*Subscription, error) {
	var from, to rpc.BlockNumber
	if crit.FromBlock == nil {
		from = rpc.LatestBlockNumber
	} else {
		from = rpc.BlockNumber(crit.FromBlock.Int64())
	}
	if crit.ToBlock == nil {
		to = rpc.LatestBlockNumber
	} else {
		to = rpc.BlockNumber(crit.ToBlock.Int64())
	}

	
	if from == rpc.PendingBlockNumber && to == rpc.PendingBlockNumber {
		return es.subscribePendingLogs(crit, logs), nil
	}
	
	if from == rpc.LatestBlockNumber && to == rpc.LatestBlockNumber {
		return es.subscribeLogs(crit, logs), nil
	}
	
	if from >= 0 && to >= 0 && to >= from {
		return es.subscribeLogs(crit, logs), nil
	}
	
	if from >= rpc.LatestBlockNumber && to == rpc.PendingBlockNumber {
		return es.subscribeMinedPendingLogs(crit, logs), nil
	}
	
	if from >= 0 && to == rpc.LatestBlockNumber {
		return es.subscribeLogs(crit, logs), nil
	}
	return nil, fmt.Errorf("invalid from and to block combination: from > to")
}



func (es *EventSystem) subscribeMinedPendingLogs(crit ethereum.FilterQuery, logs chan []*types.Log) *Subscription {
	sub := &subscription{
		id:        rpc.NewID(),
		typ:       MinedAndPendingLogsSubscription,
		logsCrit:  crit,
		created:   time.Now(),
		logs:      logs,
		hashes:    make(chan []common.Hash),
		headers:   make(chan *types.Header),
		installed: make(chan struct{}),
		err:       make(chan error),
	}
	return es.subscribe(sub)
}



func (es *EventSystem) subscribeLogs(crit ethereum.FilterQuery, logs chan []*types.Log) *Subscription {
	sub := &subscription{
		id:        rpc.NewID(),
		typ:       LogsSubscription,
		logsCrit:  crit,
		created:   time.Now(),
		logs:      logs,
		hashes:    make(chan []common.Hash),
		headers:   make(chan *types.Header),
		installed: make(chan struct{}),
		err:       make(chan error),
	}
	return es.subscribe(sub)
}



func (es *EventSystem) subscribePendingLogs(crit ethereum.FilterQuery, logs chan []*types.Log) *Subscription {
	sub := &subscription{
		id:        rpc.NewID(),
		typ:       PendingLogsSubscription,
		logsCrit:  crit,
		created:   time.Now(),
		logs:      logs,
		hashes:    make(chan []common.Hash),
		headers:   make(chan *types.Header),
		installed: make(chan struct{}),
		err:       make(chan error),
	}
	return es.subscribe(sub)
}



func (es *EventSystem) SubscribeNewHeads(headers chan *types.Header) *Subscription {
	sub := &subscription{
		id:        rpc.NewID(),
		typ:       BlocksSubscription,
		created:   time.Now(),
		logs:      make(chan []*types.Log),
		hashes:    make(chan []common.Hash),
		headers:   headers,
		installed: make(chan struct{}),
		err:       make(chan error),
	}
	return es.subscribe(sub)
}



func (es *EventSystem) SubscribePendingTxs(hashes chan []common.Hash) *Subscription {
	sub := &subscription{
		id:        rpc.NewID(),
		typ:       PendingTransactionsSubscription,
		created:   time.Now(),
		logs:      make(chan []*types.Log),
		hashes:    hashes,
		headers:   make(chan *types.Header),
		installed: make(chan struct{}),
		err:       make(chan error),
	}
	return es.subscribe(sub)
}

type filterIndex map[Type]map[rpc.ID]*subscription

func (es *EventSystem) handleLogs(filters filterIndex, ev []*types.Log) {
	if len(ev) == 0 {
		return
	}
	for _, f := range filters[LogsSubscription] {
		matchedLogs := filterLogs(ev, f.logsCrit.FromBlock, f.logsCrit.ToBlock, f.logsCrit.Addresses, f.logsCrit.Topics)
		if len(matchedLogs) > 0 {
			f.logs <- matchedLogs
		}
	}
}

func (es *EventSystem) handlePendingLogs(filters filterIndex, ev []*types.Log) {
	if len(ev) == 0 {
		return
	}
	for _, f := range filters[PendingLogsSubscription] {
		matchedLogs := filterLogs(ev, nil, f.logsCrit.ToBlock, f.logsCrit.Addresses, f.logsCrit.Topics)
		if len(matchedLogs) > 0 {
			f.logs <- matchedLogs
		}
	}
}

func (es *EventSystem) handleRemovedLogs(filters filterIndex, ev core.RemovedLogsEvent) {
	for _, f := range filters[LogsSubscription] {
		matchedLogs := filterLogs(ev.Logs, f.logsCrit.FromBlock, f.logsCrit.ToBlock, f.logsCrit.Addresses, f.logsCrit.Topics)
		if len(matchedLogs) > 0 {
			f.logs <- matchedLogs
		}
	}
}

func (es *EventSystem) handleTxsEvent(filters filterIndex, ev core.NewTxsEvent) {
	hashes := make([]common.Hash, 0, len(ev.Txs))
	for _, tx := range ev.Txs {
		hashes = append(hashes, tx.Hash())
	}
	for _, f := range filters[PendingTransactionsSubscription] {
		f.hashes <- hashes
	}
}

func (es *EventSystem) handleChainEvent(filters filterIndex, ev core.ChainEvent) {
	for _, f := range filters[BlocksSubscription] {
		f.headers <- ev.Block.Header()
	}
	if es.lightMode && len(filters[LogsSubscription]) > 0 {
		es.lightFilterNewHead(ev.Block.Header(), func(header *types.Header, remove bool) {
			for _, f := range filters[LogsSubscription] {
				if matchedLogs := es.lightFilterLogs(header, f.logsCrit.Addresses, f.logsCrit.Topics, remove); len(matchedLogs) > 0 {
					f.logs <- matchedLogs
				}
			}
		})
	}
}

func (es *EventSystem) lightFilterNewHead(newHeader *types.Header, callBack func(*types.Header, bool)) {
	oldh := es.lastHead
	es.lastHead = newHeader
	if oldh == nil {
		return
	}
	newh := newHeader
	
	var oldHeaders, newHeaders []*types.Header
	for oldh.Hash() != newh.Hash() {
		if oldh.Number.Uint64() >= newh.Number.Uint64() {
			oldHeaders = append(oldHeaders, oldh)
			oldh = rawdb.ReadHeader(es.backend.ChainDb(), oldh.ParentHash, oldh.Number.Uint64()-1)
		}
		if oldh.Number.Uint64() < newh.Number.Uint64() {
			newHeaders = append(newHeaders, newh)
			newh = rawdb.ReadHeader(es.backend.ChainDb(), newh.ParentHash, newh.Number.Uint64()-1)
			if newh == nil {
				
				newh = oldh
			}
		}
	}
	
	for _, h := range oldHeaders {
		callBack(h, true)
	}
	
	for i := len(newHeaders) - 1; i >= 0; i-- {
		callBack(newHeaders[i], false)
	}
}


func (es *EventSystem) lightFilterLogs(header *types.Header, addresses []common.Address, topics [][]common.Hash, remove bool) []*types.Log {
	if bloomFilter(header.Bloom, addresses, topics) {
		
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		logsList, err := es.backend.GetLogs(ctx, header.Hash())
		if err != nil {
			return nil
		}
		var unfiltered []*types.Log
		for _, logs := range logsList {
			for _, log := range logs {
				logcopy := *log
				logcopy.Removed = remove
				unfiltered = append(unfiltered, &logcopy)
			}
		}
		logs := filterLogs(unfiltered, nil, nil, addresses, topics)
		if len(logs) > 0 && logs[0].TxHash == (common.Hash{}) {
			
			receipts, err := es.backend.GetReceipts(ctx, header.Hash())
			if err != nil {
				return nil
			}
			unfiltered = unfiltered[:0]
			for _, receipt := range receipts {
				for _, log := range receipt.Logs {
					logcopy := *log
					logcopy.Removed = remove
					unfiltered = append(unfiltered, &logcopy)
				}
			}
			logs = filterLogs(unfiltered, nil, nil, addresses, topics)
		}
		return logs
	}
	return nil
}


func (es *EventSystem) eventLoop() {
	
	defer func() {
		es.txsSub.Unsubscribe()
		es.logsSub.Unsubscribe()
		es.rmLogsSub.Unsubscribe()
		es.pendingLogsSub.Unsubscribe()
		es.chainSub.Unsubscribe()
	}()

	index := make(filterIndex)
	for i := UnknownSubscription; i < LastIndexSubscription; i++ {
		index[i] = make(map[rpc.ID]*subscription)
	}

	for {
		select {
		case ev := <-es.txsCh:
			es.handleTxsEvent(index, ev)
		case ev := <-es.logsCh:
			es.handleLogs(index, ev)
		case ev := <-es.rmLogsCh:
			es.handleRemovedLogs(index, ev)
		case ev := <-es.pendingLogsCh:
			es.handlePendingLogs(index, ev)
		case ev := <-es.chainCh:
			es.handleChainEvent(index, ev)

		case f := <-es.install:
			if f.typ == MinedAndPendingLogsSubscription {
				
				index[LogsSubscription][f.id] = f
				index[PendingLogsSubscription][f.id] = f
			} else {
				index[f.typ][f.id] = f
			}
			close(f.installed)

		case f := <-es.uninstall:
			if f.typ == MinedAndPendingLogsSubscription {
				
				delete(index[LogsSubscription], f.id)
				delete(index[PendingLogsSubscription], f.id)
			} else {
				delete(index[f.typ], f.id)
			}
			close(f.err)

		
		case <-es.txsSub.Err():
			return
		case <-es.logsSub.Err():
			return
		case <-es.rmLogsSub.Err():
			return
		case <-es.chainSub.Err():
			return
		}
	}
}
