















package backends

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)


var _ bind.ContractBackend = (*SimulatedBackend)(nil)

var (
	errBlockNumberUnsupported  = errors.New("simulatedBackend cannot access blocks other than the latest block")
	errBlockDoesNotExist       = errors.New("block does not exist in blockchain")
	errTransactionDoesNotExist = errors.New("transaction does not exist")
)






type SimulatedBackend struct {
	database   ethdb.Database   
	blockchain *core.BlockChain 

	mu           sync.Mutex
	pendingBlock *types.Block   
	pendingState *state.StateDB 

	events *filters.EventSystem 

	config *params.ChainConfig
}



func NewSimulatedBackendWithDatabase(database ethdb.Database, alloc core.GenesisAlloc, gasLimit uint64) *SimulatedBackend {
	genesis := core.Genesis{Config: params.AllEthashProtocolChanges, GasLimit: gasLimit, Alloc: alloc}
	genesis.MustCommit(database)
	blockchain, _ := core.NewBlockChain(database, nil, genesis.Config, ethash.NewFaker(), vm.Config{}, nil, nil)

	backend := &SimulatedBackend{
		database:   database,
		blockchain: blockchain,
		config:     genesis.Config,
		events:     filters.NewEventSystem(&filterBackend{database, blockchain}, false),
	}
	backend.rollback()
	return backend
}



func NewSimulatedBackend(alloc core.GenesisAlloc, gasLimit uint64) *SimulatedBackend {
	return NewSimulatedBackendWithDatabase(rawdb.NewMemoryDatabase(), alloc, gasLimit)
}


func (b *SimulatedBackend) Close() error {
	b.blockchain.Stop()
	return nil
}



func (b *SimulatedBackend) Commit() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := b.blockchain.InsertChain([]*types.Block{b.pendingBlock}); err != nil {
		panic(err) 
	}
	b.rollback()
}


func (b *SimulatedBackend) Rollback() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.rollback()
}

func (b *SimulatedBackend) rollback() {
	blocks, _ := core.GenerateChain(b.config, b.blockchain.CurrentBlock(), ethash.NewFaker(), b.database, 1, func(int, *core.BlockGen) {})
	stateDB, _ := b.blockchain.State()

	b.pendingBlock = blocks[0]
	b.pendingState, _ = state.New(b.pendingBlock.Root(), stateDB.Database(), nil)
}


func (b *SimulatedBackend) stateByBlockNumber(ctx context.Context, blockNumber *big.Int) (*state.StateDB, error) {
	if blockNumber == nil || blockNumber.Cmp(b.blockchain.CurrentBlock().Number()) == 0 {
		return b.blockchain.State()
	}
	block, err := b.blockByNumberNoLock(ctx, blockNumber)
	if err != nil {
		return nil, err
	}
	return b.blockchain.StateAt(block.Root())
}


func (b *SimulatedBackend) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	stateDB, err := b.stateByBlockNumber(ctx, blockNumber)
	if err != nil {
		return nil, err
	}

	return stateDB.GetCode(contract), nil
}


func (b *SimulatedBackend) BalanceAt(ctx context.Context, contract common.Address, blockNumber *big.Int) (*big.Int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	stateDB, err := b.stateByBlockNumber(ctx, blockNumber)
	if err != nil {
		return nil, err
	}

	return stateDB.GetBalance(contract), nil
}


func (b *SimulatedBackend) NonceAt(ctx context.Context, contract common.Address, blockNumber *big.Int) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	stateDB, err := b.stateByBlockNumber(ctx, blockNumber)
	if err != nil {
		return 0, err
	}

	return stateDB.GetNonce(contract), nil
}


func (b *SimulatedBackend) StorageAt(ctx context.Context, contract common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	stateDB, err := b.stateByBlockNumber(ctx, blockNumber)
	if err != nil {
		return nil, err
	}

	val := stateDB.GetState(contract, key)
	return val[:], nil
}


func (b *SimulatedBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	receipt, _, _, _ := rawdb.ReadReceipt(b.database, txHash, b.config)
	return receipt, nil
}





func (b *SimulatedBackend) TransactionByHash(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	tx := b.pendingBlock.Transaction(txHash)
	if tx != nil {
		return tx, true, nil
	}
	tx, _, _, _ = rawdb.ReadTransaction(b.database, txHash)
	if tx != nil {
		return tx, false, nil
	}
	return nil, false, ethereum.NotFound
}


func (b *SimulatedBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if hash == b.pendingBlock.Hash() {
		return b.pendingBlock, nil
	}

	block := b.blockchain.GetBlockByHash(hash)
	if block != nil {
		return block, nil
	}

	return nil, errBlockDoesNotExist
}



func (b *SimulatedBackend) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.blockByNumberNoLock(ctx, number)
}



func (b *SimulatedBackend) blockByNumberNoLock(ctx context.Context, number *big.Int) (*types.Block, error) {
	if number == nil || number.Cmp(b.pendingBlock.Number()) == 0 {
		return b.blockchain.CurrentBlock(), nil
	}

	block := b.blockchain.GetBlockByNumber(uint64(number.Int64()))
	if block == nil {
		return nil, errBlockDoesNotExist
	}

	return block, nil
}


func (b *SimulatedBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if hash == b.pendingBlock.Hash() {
		return b.pendingBlock.Header(), nil
	}

	header := b.blockchain.GetHeaderByHash(hash)
	if header == nil {
		return nil, errBlockDoesNotExist
	}

	return header, nil
}



func (b *SimulatedBackend) HeaderByNumber(ctx context.Context, block *big.Int) (*types.Header, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if block == nil || block.Cmp(b.pendingBlock.Number()) == 0 {
		return b.blockchain.CurrentHeader(), nil
	}

	return b.blockchain.GetHeaderByNumber(uint64(block.Int64())), nil
}


func (b *SimulatedBackend) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if blockHash == b.pendingBlock.Hash() {
		return uint(b.pendingBlock.Transactions().Len()), nil
	}

	block := b.blockchain.GetBlockByHash(blockHash)
	if block == nil {
		return uint(0), errBlockDoesNotExist
	}

	return uint(block.Transactions().Len()), nil
}


func (b *SimulatedBackend) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if blockHash == b.pendingBlock.Hash() {
		transactions := b.pendingBlock.Transactions()
		if uint(len(transactions)) < index+1 {
			return nil, errTransactionDoesNotExist
		}

		return transactions[index], nil
	}

	block := b.blockchain.GetBlockByHash(blockHash)
	if block == nil {
		return nil, errBlockDoesNotExist
	}

	transactions := block.Transactions()
	if uint(len(transactions)) < index+1 {
		return nil, errTransactionDoesNotExist
	}

	return transactions[index], nil
}


func (b *SimulatedBackend) PendingCodeAt(ctx context.Context, contract common.Address) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.pendingState.GetCode(contract), nil
}

func newRevertError(result *core.ExecutionResult) *revertError {
	reason, errUnpack := abi.UnpackRevert(result.Revert())
	err := errors.New("execution reverted")
	if errUnpack == nil {
		err = fmt.Errorf("execution reverted: %v", reason)
	}
	return &revertError{
		error:  err,
		reason: hexutil.Encode(result.Revert()),
	}
}



type revertError struct {
	error
	reason string 
}



func (e *revertError) ErrorCode() int {
	return 3
}


func (e *revertError) ErrorData() interface{} {
	return e.reason
}


func (b *SimulatedBackend) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if blockNumber != nil && blockNumber.Cmp(b.blockchain.CurrentBlock().Number()) != 0 {
		return nil, errBlockNumberUnsupported
	}
	stateDB, err := b.blockchain.State()
	if err != nil {
		return nil, err
	}
	res, err := b.callContract(ctx, call, b.blockchain.CurrentBlock(), stateDB)
	if err != nil {
		return nil, err
	}
	
	if len(res.Revert()) > 0 {
		return nil, newRevertError(res)
	}
	return res.Return(), res.Err
}


func (b *SimulatedBackend) PendingCallContract(ctx context.Context, call ethereum.CallMsg) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	defer b.pendingState.RevertToSnapshot(b.pendingState.Snapshot())

	res, err := b.callContract(ctx, call, b.pendingBlock, b.pendingState)
	if err != nil {
		return nil, err
	}
	
	if len(res.Revert()) > 0 {
		return nil, newRevertError(res)
	}
	return res.Return(), res.Err
}



func (b *SimulatedBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.pendingState.GetOrNewStateObject(account).Nonce(), nil
}



func (b *SimulatedBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}



func (b *SimulatedBackend) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	
	var (
		lo  uint64 = params.TxGas - 1
		hi  uint64
		cap uint64
	)
	if call.Gas >= params.TxGas {
		hi = call.Gas
	} else {
		hi = b.pendingBlock.GasLimit()
	}
	
	if call.GasPrice != nil && call.GasPrice.BitLen() != 0 {
		balance := b.pendingState.GetBalance(call.From) 
		available := new(big.Int).Set(balance)
		if call.Value != nil {
			if call.Value.Cmp(available) >= 0 {
				return 0, errors.New("insufficient funds for transfer")
			}
			available.Sub(available, call.Value)
		}
		allowance := new(big.Int).Div(available, call.GasPrice)
		if allowance.IsUint64() && hi > allowance.Uint64() {
			transfer := call.Value
			if transfer == nil {
				transfer = new(big.Int)
			}
			log.Warn("Gas estimation capped by limited funds", "original", hi, "balance", balance,
				"sent", transfer, "gasprice", call.GasPrice, "fundable", allowance)
			hi = allowance.Uint64()
		}
	}
	cap = hi

	
	executable := func(gas uint64) (bool, *core.ExecutionResult, error) {
		call.Gas = gas

		snapshot := b.pendingState.Snapshot()
		res, err := b.callContract(ctx, call, b.pendingBlock, b.pendingState)
		b.pendingState.RevertToSnapshot(snapshot)

		if err != nil {
			if err == core.ErrIntrinsicGas {
				return true, nil, nil 
			}
			return true, nil, err 
		}
		return res.Failed(), res, nil
	}
	
	for lo+1 < hi {
		mid := (hi + lo) / 2
		failed, _, err := executable(mid)

		
		
		
		if err != nil {
			return 0, err
		}
		if failed {
			lo = mid
		} else {
			hi = mid
		}
	}
	
	if hi == cap {
		failed, result, err := executable(hi)
		if err != nil {
			return 0, err
		}
		if failed {
			if result != nil && result.Err != vm.ErrOutOfGas {
				if len(result.Revert()) > 0 {
					return 0, newRevertError(result)
				}
				return 0, result.Err
			}
			
			return 0, fmt.Errorf("gas required exceeds allowance (%d)", cap)
		}
	}
	return hi, nil
}



func (b *SimulatedBackend) callContract(ctx context.Context, call ethereum.CallMsg, block *types.Block, stateDB *state.StateDB) (*core.ExecutionResult, error) {
	
	if call.GasPrice == nil {
		call.GasPrice = big.NewInt(1)
	}
	if call.Gas == 0 {
		call.Gas = 50000000
	}
	if call.Value == nil {
		call.Value = new(big.Int)
	}
	
	from := stateDB.GetOrNewStateObject(call.From)
	from.SetBalance(math.MaxBig256)
	
	msg := callMsg{call}

	evmContext := core.NewEVMContext(msg, block.Header(), b.blockchain, nil)
	
	
	vmEnv := vm.NewEVM(evmContext, stateDB, b.config, vm.Config{})
	gasPool := new(core.GasPool).AddGas(math.MaxUint64)

	return core.NewStateTransition(vmEnv, msg, gasPool).TransitionDb()
}



func (b *SimulatedBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	sender, err := types.Sender(types.NewEIP155Signer(b.config.ChainID), tx)
	if err != nil {
		panic(fmt.Errorf("invalid transaction: %v", err))
	}
	nonce := b.pendingState.GetNonce(sender)
	if tx.Nonce() != nonce {
		panic(fmt.Errorf("invalid transaction nonce: got %d, want %d", tx.Nonce(), nonce))
	}

	blocks, _ := core.GenerateChain(b.config, b.blockchain.CurrentBlock(), ethash.NewFaker(), b.database, 1, func(number int, block *core.BlockGen) {
		for _, tx := range b.pendingBlock.Transactions() {
			block.AddTxWithChain(b.blockchain, tx)
		}
		block.AddTxWithChain(b.blockchain, tx)
	})
	stateDB, _ := b.blockchain.State()

	b.pendingBlock = blocks[0]
	b.pendingState, _ = state.New(b.pendingBlock.Root(), stateDB.Database(), nil)
	return nil
}





func (b *SimulatedBackend) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	var filter *filters.Filter
	if query.BlockHash != nil {
		
		filter = filters.NewBlockFilter(&filterBackend{b.database, b.blockchain}, *query.BlockHash, query.Addresses, query.Topics)
	} else {
		
		from := int64(0)
		if query.FromBlock != nil {
			from = query.FromBlock.Int64()
		}
		to := int64(-1)
		if query.ToBlock != nil {
			to = query.ToBlock.Int64()
		}
		
		filter = filters.NewRangeFilter(&filterBackend{b.database, b.blockchain}, from, to, query.Addresses, query.Topics)
	}
	
	logs, err := filter.Logs(ctx)
	if err != nil {
		return nil, err
	}
	res := make([]types.Log, len(logs))
	for i, nLog := range logs {
		res[i] = *nLog
	}
	return res, nil
}



func (b *SimulatedBackend) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	
	sink := make(chan []*types.Log)

	sub, err := b.events.SubscribeLogs(query, sink)
	if err != nil {
		return nil, err
	}
	
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case logs := <-sink:
				for _, nlog := range logs {
					select {
					case ch <- *nlog:
					case err := <-sub.Err():
						return err
					case <-quit:
						return nil
					}
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}


func (b *SimulatedBackend) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	
	sink := make(chan *types.Header)
	sub := b.events.SubscribeNewHeads(sink)

	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case head := <-sink:
				select {
				case ch <- head:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}



func (b *SimulatedBackend) AdjustTime(adjustment time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.pendingBlock.Transactions()) != 0 {
		return errors.New("Could not adjust time on non-empty block")
	}

	blocks, _ := core.GenerateChain(b.config, b.blockchain.CurrentBlock(), ethash.NewFaker(), b.database, 1, func(number int, block *core.BlockGen) {
		block.OffsetTime(int64(adjustment.Seconds()))
	})
	stateDB, _ := b.blockchain.State()

	b.pendingBlock = blocks[0]
	b.pendingState, _ = state.New(b.pendingBlock.Root(), stateDB.Database(), nil)

	return nil
}


func (b *SimulatedBackend) Blockchain() *core.BlockChain {
	return b.blockchain
}


type callMsg struct {
	ethereum.CallMsg
}

func (m callMsg) From() common.Address { return m.CallMsg.From }
func (m callMsg) Nonce() uint64        { return 0 }
func (m callMsg) CheckNonce() bool     { return false }
func (m callMsg) To() *common.Address  { return m.CallMsg.To }
func (m callMsg) GasPrice() *big.Int   { return m.CallMsg.GasPrice }
func (m callMsg) Gas() uint64          { return m.CallMsg.Gas }
func (m callMsg) Value() *big.Int      { return m.CallMsg.Value }
func (m callMsg) Data() []byte         { return m.CallMsg.Data }



type filterBackend struct {
	db ethdb.Database
	bc *core.BlockChain
}

func (fb *filterBackend) ChainDb() ethdb.Database  { return fb.db }
func (fb *filterBackend) EventMux() *event.TypeMux { panic("not supported") }

func (fb *filterBackend) HeaderByNumber(ctx context.Context, block rpc.BlockNumber) (*types.Header, error) {
	if block == rpc.LatestBlockNumber {
		return fb.bc.CurrentHeader(), nil
	}
	return fb.bc.GetHeaderByNumber(uint64(block.Int64())), nil
}

func (fb *filterBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return fb.bc.GetHeaderByHash(hash), nil
}

func (fb *filterBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	number := rawdb.ReadHeaderNumber(fb.db, hash)
	if number == nil {
		return nil, nil
	}
	return rawdb.ReadReceipts(fb.db, hash, *number, fb.bc.Config()), nil
}

func (fb *filterBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	number := rawdb.ReadHeaderNumber(fb.db, hash)
	if number == nil {
		return nil, nil
	}
	receipts := rawdb.ReadReceipts(fb.db, hash, *number, fb.bc.Config())
	if receipts == nil {
		return nil, nil
	}
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (fb *filterBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return nullSubscription()
}

func (fb *filterBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return fb.bc.SubscribeChainEvent(ch)
}

func (fb *filterBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return fb.bc.SubscribeRemovedLogsEvent(ch)
}

func (fb *filterBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return fb.bc.SubscribeLogsEvent(ch)
}

func (fb *filterBackend) SubscribePendingLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return nullSubscription()
}

func (fb *filterBackend) BloomStatus() (uint64, uint64) { return 4096, 0 }

func (fb *filterBackend) ServiceFilter(ctx context.Context, ms *bloombits.MatcherSession) {
	panic("not supported")
}

func nullSubscription() event.Subscription {
	return event.NewSubscription(func(quit <-chan struct{}) error {
		<-quit
		return nil
	})
}
