
















package graphql

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	errBlockInvariant = errors.New("block objects must be instantiated with at least one of num or hash")
)


type Account struct {
	backend       ethapi.Backend
	address       common.Address
	blockNrOrHash rpc.BlockNumberOrHash
}


func (a *Account) getState(ctx context.Context) (*state.StateDB, error) {
	state, _, err := a.backend.StateAndHeaderByNumberOrHash(ctx, a.blockNrOrHash)
	return state, err
}

func (a *Account) Address(ctx context.Context) (common.Address, error) {
	return a.address, nil
}

func (a *Account) Balance(ctx context.Context) (hexutil.Big, error) {
	state, err := a.getState(ctx)
	if err != nil {
		return hexutil.Big{}, err
	}
	return hexutil.Big(*state.GetBalance(a.address)), nil
}

func (a *Account) TransactionCount(ctx context.Context) (hexutil.Uint64, error) {
	state, err := a.getState(ctx)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(state.GetNonce(a.address)), nil
}

func (a *Account) Code(ctx context.Context) (hexutil.Bytes, error) {
	state, err := a.getState(ctx)
	if err != nil {
		return hexutil.Bytes{}, err
	}
	return hexutil.Bytes(state.GetCode(a.address)), nil
}

func (a *Account) Storage(ctx context.Context, args struct{ Slot common.Hash }) (common.Hash, error) {
	state, err := a.getState(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return state.GetState(a.address, args.Slot), nil
}


type Log struct {
	backend     ethapi.Backend
	transaction *Transaction
	log         *types.Log
}

func (l *Log) Transaction(ctx context.Context) *Transaction {
	return l.transaction
}

func (l *Log) Account(ctx context.Context, args BlockNumberArgs) *Account {
	return &Account{
		backend:       l.backend,
		address:       l.log.Address,
		blockNrOrHash: args.NumberOrLatest(),
	}
}

func (l *Log) Index(ctx context.Context) int32 {
	return int32(l.log.Index)
}

func (l *Log) Topics(ctx context.Context) []common.Hash {
	return l.log.Topics
}

func (l *Log) Data(ctx context.Context) hexutil.Bytes {
	return hexutil.Bytes(l.log.Data)
}



type Transaction struct {
	backend ethapi.Backend
	hash    common.Hash
	tx      *types.Transaction
	block   *Block
	index   uint64
}


func (t *Transaction) resolve(ctx context.Context) (*types.Transaction, error) {
	if t.tx == nil {
		tx, blockHash, _, index := rawdb.ReadTransaction(t.backend.ChainDb(), t.hash)
		if tx != nil {
			t.tx = tx
			blockNrOrHash := rpc.BlockNumberOrHashWithHash(blockHash, false)
			t.block = &Block{
				backend:      t.backend,
				numberOrHash: &blockNrOrHash,
			}
			t.index = index
		} else {
			t.tx = t.backend.GetPoolTransaction(t.hash)
		}
	}
	return t.tx, nil
}

func (t *Transaction) Hash(ctx context.Context) common.Hash {
	return t.hash
}

func (t *Transaction) InputData(ctx context.Context) (hexutil.Bytes, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return hexutil.Bytes{}, err
	}
	return hexutil.Bytes(tx.Data()), nil
}

func (t *Transaction) Gas(ctx context.Context) (hexutil.Uint64, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return 0, err
	}
	return hexutil.Uint64(tx.Gas()), nil
}

func (t *Transaction) GasPrice(ctx context.Context) (hexutil.Big, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return hexutil.Big{}, err
	}
	return hexutil.Big(*tx.GasPrice()), nil
}

func (t *Transaction) Value(ctx context.Context) (hexutil.Big, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return hexutil.Big{}, err
	}
	return hexutil.Big(*tx.Value()), nil
}

func (t *Transaction) Nonce(ctx context.Context) (hexutil.Uint64, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return 0, err
	}
	return hexutil.Uint64(tx.Nonce()), nil
}

func (t *Transaction) To(ctx context.Context, args BlockNumberArgs) (*Account, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return nil, err
	}
	to := tx.To()
	if to == nil {
		return nil, nil
	}
	return &Account{
		backend:       t.backend,
		address:       *to,
		blockNrOrHash: args.NumberOrLatest(),
	}, nil
}

func (t *Transaction) From(ctx context.Context, args BlockNumberArgs) (*Account, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return nil, err
	}
	var signer types.Signer = types.HomesteadSigner{}
	if tx.Protected() {
		signer = types.NewEIP155Signer(tx.ChainId())
	}
	from, _ := types.Sender(signer, tx)

	return &Account{
		backend:       t.backend,
		address:       from,
		blockNrOrHash: args.NumberOrLatest(),
	}, nil
}

func (t *Transaction) Block(ctx context.Context) (*Block, error) {
	if _, err := t.resolve(ctx); err != nil {
		return nil, err
	}
	return t.block, nil
}

func (t *Transaction) Index(ctx context.Context) (*int32, error) {
	if _, err := t.resolve(ctx); err != nil {
		return nil, err
	}
	if t.block == nil {
		return nil, nil
	}
	index := int32(t.index)
	return &index, nil
}


func (t *Transaction) getReceipt(ctx context.Context) (*types.Receipt, error) {
	if _, err := t.resolve(ctx); err != nil {
		return nil, err
	}
	if t.block == nil {
		return nil, nil
	}
	receipts, err := t.block.resolveReceipts(ctx)
	if err != nil {
		return nil, err
	}
	return receipts[t.index], nil
}

func (t *Transaction) Status(ctx context.Context) (*hexutil.Uint64, error) {
	receipt, err := t.getReceipt(ctx)
	if err != nil || receipt == nil {
		return nil, err
	}
	ret := hexutil.Uint64(receipt.Status)
	return &ret, nil
}

func (t *Transaction) GasUsed(ctx context.Context) (*hexutil.Uint64, error) {
	receipt, err := t.getReceipt(ctx)
	if err != nil || receipt == nil {
		return nil, err
	}
	ret := hexutil.Uint64(receipt.GasUsed)
	return &ret, nil
}

func (t *Transaction) CumulativeGasUsed(ctx context.Context) (*hexutil.Uint64, error) {
	receipt, err := t.getReceipt(ctx)
	if err != nil || receipt == nil {
		return nil, err
	}
	ret := hexutil.Uint64(receipt.CumulativeGasUsed)
	return &ret, nil
}

func (t *Transaction) CreatedContract(ctx context.Context, args BlockNumberArgs) (*Account, error) {
	receipt, err := t.getReceipt(ctx)
	if err != nil || receipt == nil || receipt.ContractAddress == (common.Address{}) {
		return nil, err
	}
	return &Account{
		backend:       t.backend,
		address:       receipt.ContractAddress,
		blockNrOrHash: args.NumberOrLatest(),
	}, nil
}

func (t *Transaction) Logs(ctx context.Context) (*[]*Log, error) {
	receipt, err := t.getReceipt(ctx)
	if err != nil || receipt == nil {
		return nil, err
	}
	ret := make([]*Log, 0, len(receipt.Logs))
	for _, log := range receipt.Logs {
		ret = append(ret, &Log{
			backend:     t.backend,
			transaction: t,
			log:         log,
		})
	}
	return &ret, nil
}

func (t *Transaction) R(ctx context.Context) (hexutil.Big, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return hexutil.Big{}, err
	}
	_, r, _ := tx.RawSignatureValues()
	return hexutil.Big(*r), nil
}

func (t *Transaction) S(ctx context.Context) (hexutil.Big, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return hexutil.Big{}, err
	}
	_, _, s := tx.RawSignatureValues()
	return hexutil.Big(*s), nil
}

func (t *Transaction) V(ctx context.Context) (hexutil.Big, error) {
	tx, err := t.resolve(ctx)
	if err != nil || tx == nil {
		return hexutil.Big{}, err
	}
	v, _, _ := tx.RawSignatureValues()
	return hexutil.Big(*v), nil
}

type BlockType int




type Block struct {
	backend      ethapi.Backend
	numberOrHash *rpc.BlockNumberOrHash
	hash         common.Hash
	header       *types.Header
	block        *types.Block
	receipts     []*types.Receipt
}



func (b *Block) resolve(ctx context.Context) (*types.Block, error) {
	if b.block != nil {
		return b.block, nil
	}
	if b.numberOrHash == nil {
		latest := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
		b.numberOrHash = &latest
	}
	var err error
	b.block, err = b.backend.BlockByNumberOrHash(ctx, *b.numberOrHash)
	if b.block != nil && b.header == nil {
		b.header = b.block.Header()
		if hash, ok := b.numberOrHash.Hash(); ok {
			b.hash = hash
		}
	}
	return b.block, err
}




func (b *Block) resolveHeader(ctx context.Context) (*types.Header, error) {
	if b.numberOrHash == nil && b.hash == (common.Hash{}) {
		return nil, errBlockInvariant
	}
	var err error
	if b.header == nil {
		if b.hash != (common.Hash{}) {
			b.header, err = b.backend.HeaderByHash(ctx, b.hash)
		} else {
			b.header, err = b.backend.HeaderByNumberOrHash(ctx, *b.numberOrHash)
		}
	}
	return b.header, err
}



func (b *Block) resolveReceipts(ctx context.Context) ([]*types.Receipt, error) {
	if b.receipts == nil {
		hash := b.hash
		if hash == (common.Hash{}) {
			header, err := b.resolveHeader(ctx)
			if err != nil {
				return nil, err
			}
			hash = header.Hash()
		}
		receipts, err := b.backend.GetReceipts(ctx, hash)
		if err != nil {
			return nil, err
		}
		b.receipts = []*types.Receipt(receipts)
	}
	return b.receipts, nil
}

func (b *Block) Number(ctx context.Context) (hexutil.Uint64, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return 0, err
	}

	return hexutil.Uint64(header.Number.Uint64()), nil
}

func (b *Block) Hash(ctx context.Context) (common.Hash, error) {
	if b.hash == (common.Hash{}) {
		header, err := b.resolveHeader(ctx)
		if err != nil {
			return common.Hash{}, err
		}
		b.hash = header.Hash()
	}
	return b.hash, nil
}

func (b *Block) GasLimit(ctx context.Context) (hexutil.Uint64, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(header.GasLimit), nil
}

func (b *Block) GasUsed(ctx context.Context) (hexutil.Uint64, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(header.GasUsed), nil
}

func (b *Block) Parent(ctx context.Context) (*Block, error) {
	
	if b.numberOrHash == nil && b.header == nil {
		if _, err := b.resolveHeader(ctx); err != nil {
			return nil, err
		}
	}
	if b.header != nil && b.header.Number.Uint64() > 0 {
		num := rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(b.header.Number.Uint64() - 1))
		return &Block{
			backend:      b.backend,
			numberOrHash: &num,
			hash:         b.header.ParentHash,
		}, nil
	}
	return nil, nil
}

func (b *Block) Difficulty(ctx context.Context) (hexutil.Big, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return hexutil.Big{}, err
	}
	return hexutil.Big(*header.Difficulty), nil
}

func (b *Block) Timestamp(ctx context.Context) (hexutil.Uint64, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(header.Time), nil
}

func (b *Block) Nonce(ctx context.Context) (hexutil.Bytes, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return hexutil.Bytes{}, err
	}
	return hexutil.Bytes(header.Nonce[:]), nil
}

func (b *Block) MixHash(ctx context.Context) (common.Hash, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return header.MixDigest, nil
}

func (b *Block) TransactionsRoot(ctx context.Context) (common.Hash, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return header.TxHash, nil
}

func (b *Block) StateRoot(ctx context.Context) (common.Hash, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return header.Root, nil
}

func (b *Block) ReceiptsRoot(ctx context.Context) (common.Hash, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return header.ReceiptHash, nil
}

func (b *Block) OmmerHash(ctx context.Context) (common.Hash, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	return header.UncleHash, nil
}

func (b *Block) OmmerCount(ctx context.Context) (*int32, error) {
	block, err := b.resolve(ctx)
	if err != nil || block == nil {
		return nil, err
	}
	count := int32(len(block.Uncles()))
	return &count, err
}

func (b *Block) Ommers(ctx context.Context) (*[]*Block, error) {
	block, err := b.resolve(ctx)
	if err != nil || block == nil {
		return nil, err
	}
	ret := make([]*Block, 0, len(block.Uncles()))
	for _, uncle := range block.Uncles() {
		blockNumberOrHash := rpc.BlockNumberOrHashWithHash(uncle.Hash(), false)
		ret = append(ret, &Block{
			backend:      b.backend,
			numberOrHash: &blockNumberOrHash,
			header:       uncle,
		})
	}
	return &ret, nil
}

func (b *Block) ExtraData(ctx context.Context) (hexutil.Bytes, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return hexutil.Bytes{}, err
	}
	return hexutil.Bytes(header.Extra), nil
}

func (b *Block) LogsBloom(ctx context.Context) (hexutil.Bytes, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return hexutil.Bytes{}, err
	}
	return hexutil.Bytes(header.Bloom.Bytes()), nil
}

func (b *Block) TotalDifficulty(ctx context.Context) (hexutil.Big, error) {
	h := b.hash
	if h == (common.Hash{}) {
		header, err := b.resolveHeader(ctx)
		if err != nil {
			return hexutil.Big{}, err
		}
		h = header.Hash()
	}
	return hexutil.Big(*b.backend.GetTd(ctx, h)), nil
}


type BlockNumberArgs struct {
	
	
	
	Block *hexutil.Uint64
}



func (a BlockNumberArgs) NumberOr(current rpc.BlockNumberOrHash) rpc.BlockNumberOrHash {
	if a.Block != nil {
		blockNr := rpc.BlockNumber(*a.Block)
		return rpc.BlockNumberOrHashWithNumber(blockNr)
	}
	return current
}



func (a BlockNumberArgs) NumberOrLatest() rpc.BlockNumberOrHash {
	return a.NumberOr(rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber))
}

func (b *Block) Miner(ctx context.Context, args BlockNumberArgs) (*Account, error) {
	header, err := b.resolveHeader(ctx)
	if err != nil {
		return nil, err
	}
	return &Account{
		backend:       b.backend,
		address:       header.Coinbase,
		blockNrOrHash: args.NumberOrLatest(),
	}, nil
}

func (b *Block) TransactionCount(ctx context.Context) (*int32, error) {
	block, err := b.resolve(ctx)
	if err != nil || block == nil {
		return nil, err
	}
	count := int32(len(block.Transactions()))
	return &count, err
}

func (b *Block) Transactions(ctx context.Context) (*[]*Transaction, error) {
	block, err := b.resolve(ctx)
	if err != nil || block == nil {
		return nil, err
	}
	ret := make([]*Transaction, 0, len(block.Transactions()))
	for i, tx := range block.Transactions() {
		ret = append(ret, &Transaction{
			backend: b.backend,
			hash:    tx.Hash(),
			tx:      tx,
			block:   b,
			index:   uint64(i),
		})
	}
	return &ret, nil
}

func (b *Block) TransactionAt(ctx context.Context, args struct{ Index int32 }) (*Transaction, error) {
	block, err := b.resolve(ctx)
	if err != nil || block == nil {
		return nil, err
	}
	txs := block.Transactions()
	if args.Index < 0 || int(args.Index) >= len(txs) {
		return nil, nil
	}
	tx := txs[args.Index]
	return &Transaction{
		backend: b.backend,
		hash:    tx.Hash(),
		tx:      tx,
		block:   b,
		index:   uint64(args.Index),
	}, nil
}

func (b *Block) OmmerAt(ctx context.Context, args struct{ Index int32 }) (*Block, error) {
	block, err := b.resolve(ctx)
	if err != nil || block == nil {
		return nil, err
	}
	uncles := block.Uncles()
	if args.Index < 0 || int(args.Index) >= len(uncles) {
		return nil, nil
	}
	uncle := uncles[args.Index]
	blockNumberOrHash := rpc.BlockNumberOrHashWithHash(uncle.Hash(), false)
	return &Block{
		backend:      b.backend,
		numberOrHash: &blockNumberOrHash,
		header:       uncle,
	}, nil
}



type BlockFilterCriteria struct {
	Addresses *[]common.Address 

	
	
	
	
	
	
	
	
	
	
	
	Topics *[][]common.Hash
}



func runFilter(ctx context.Context, be ethapi.Backend, filter *filters.Filter) ([]*Log, error) {
	logs, err := filter.Logs(ctx)
	if err != nil || logs == nil {
		return nil, err
	}
	ret := make([]*Log, 0, len(logs))
	for _, log := range logs {
		ret = append(ret, &Log{
			backend:     be,
			transaction: &Transaction{backend: be, hash: log.TxHash},
			log:         log,
		})
	}
	return ret, nil
}

func (b *Block) Logs(ctx context.Context, args struct{ Filter BlockFilterCriteria }) ([]*Log, error) {
	var addresses []common.Address
	if args.Filter.Addresses != nil {
		addresses = *args.Filter.Addresses
	}
	var topics [][]common.Hash
	if args.Filter.Topics != nil {
		topics = *args.Filter.Topics
	}
	hash := b.hash
	if hash == (common.Hash{}) {
		header, err := b.resolveHeader(ctx)
		if err != nil {
			return nil, err
		}
		hash = header.Hash()
	}
	
	filter := filters.NewBlockFilter(b.backend, hash, addresses, topics)

	
	return runFilter(ctx, b.backend, filter)
}

func (b *Block) Account(ctx context.Context, args struct {
	Address common.Address
}) (*Account, error) {
	if b.numberOrHash == nil {
		_, err := b.resolveHeader(ctx)
		if err != nil {
			return nil, err
		}
	}
	return &Account{
		backend:       b.backend,
		address:       args.Address,
		blockNrOrHash: *b.numberOrHash,
	}, nil
}



type CallData struct {
	From     *common.Address 
	To       *common.Address 
	Gas      *hexutil.Uint64 
	GasPrice *hexutil.Big    
	Value    *hexutil.Big    
	Data     *hexutil.Bytes  
}


type CallResult struct {
	data    hexutil.Bytes  
	gasUsed hexutil.Uint64 
	status  hexutil.Uint64 
}

func (c *CallResult) Data() hexutil.Bytes {
	return c.data
}

func (c *CallResult) GasUsed() hexutil.Uint64 {
	return c.gasUsed
}

func (c *CallResult) Status() hexutil.Uint64 {
	return c.status
}

func (b *Block) Call(ctx context.Context, args struct {
	Data ethapi.CallArgs
}) (*CallResult, error) {
	if b.numberOrHash == nil {
		_, err := b.resolve(ctx)
		if err != nil {
			return nil, err
		}
	}
	result, err := ethapi.DoCall(ctx, b.backend, args.Data, *b.numberOrHash, nil, vm.Config{}, 5*time.Second, b.backend.RPCGasCap())
	if err != nil {
		return nil, err
	}
	status := hexutil.Uint64(1)
	if result.Failed() {
		status = 0
	}

	return &CallResult{
		data:    result.ReturnData,
		gasUsed: hexutil.Uint64(result.UsedGas),
		status:  status,
	}, nil
}

func (b *Block) EstimateGas(ctx context.Context, args struct {
	Data ethapi.CallArgs
}) (hexutil.Uint64, error) {
	if b.numberOrHash == nil {
		_, err := b.resolveHeader(ctx)
		if err != nil {
			return hexutil.Uint64(0), err
		}
	}
	gas, err := ethapi.DoEstimateGas(ctx, b.backend, args.Data, *b.numberOrHash, b.backend.RPCGasCap())
	return gas, err
}

type Pending struct {
	backend ethapi.Backend
}

func (p *Pending) TransactionCount(ctx context.Context) (int32, error) {
	txs, err := p.backend.GetPoolTransactions()
	return int32(len(txs)), err
}

func (p *Pending) Transactions(ctx context.Context) (*[]*Transaction, error) {
	txs, err := p.backend.GetPoolTransactions()
	if err != nil {
		return nil, err
	}
	ret := make([]*Transaction, 0, len(txs))
	for i, tx := range txs {
		ret = append(ret, &Transaction{
			backend: p.backend,
			hash:    tx.Hash(),
			tx:      tx,
			index:   uint64(i),
		})
	}
	return &ret, nil
}

func (p *Pending) Account(ctx context.Context, args struct {
	Address common.Address
}) *Account {
	pendingBlockNr := rpc.BlockNumberOrHashWithNumber(rpc.PendingBlockNumber)
	return &Account{
		backend:       p.backend,
		address:       args.Address,
		blockNrOrHash: pendingBlockNr,
	}
}

func (p *Pending) Call(ctx context.Context, args struct {
	Data ethapi.CallArgs
}) (*CallResult, error) {
	pendingBlockNr := rpc.BlockNumberOrHashWithNumber(rpc.PendingBlockNumber)
	result, err := ethapi.DoCall(ctx, p.backend, args.Data, pendingBlockNr, nil, vm.Config{}, 5*time.Second, p.backend.RPCGasCap())
	if err != nil {
		return nil, err
	}
	status := hexutil.Uint64(1)
	if result.Failed() {
		status = 0
	}

	return &CallResult{
		data:    result.ReturnData,
		gasUsed: hexutil.Uint64(result.UsedGas),
		status:  status,
	}, nil
}

func (p *Pending) EstimateGas(ctx context.Context, args struct {
	Data ethapi.CallArgs
}) (hexutil.Uint64, error) {
	pendingBlockNr := rpc.BlockNumberOrHashWithNumber(rpc.PendingBlockNumber)
	return ethapi.DoEstimateGas(ctx, p.backend, args.Data, pendingBlockNr, p.backend.RPCGasCap())
}


type Resolver struct {
	backend ethapi.Backend
}

func (r *Resolver) Block(ctx context.Context, args struct {
	Number *hexutil.Uint64
	Hash   *common.Hash
}) (*Block, error) {
	var block *Block
	if args.Number != nil {
		number := rpc.BlockNumber(uint64(*args.Number))
		numberOrHash := rpc.BlockNumberOrHashWithNumber(number)
		block = &Block{
			backend:      r.backend,
			numberOrHash: &numberOrHash,
		}
	} else if args.Hash != nil {
		numberOrHash := rpc.BlockNumberOrHashWithHash(*args.Hash, false)
		block = &Block{
			backend:      r.backend,
			numberOrHash: &numberOrHash,
		}
	} else {
		numberOrHash := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
		block = &Block{
			backend:      r.backend,
			numberOrHash: &numberOrHash,
		}
	}
	
	
	
	h, err := block.resolveHeader(ctx)
	if err != nil {
		return nil, err
	} else if h == nil {
		return nil, nil
	}
	return block, nil
}

func (r *Resolver) Blocks(ctx context.Context, args struct {
	From hexutil.Uint64
	To   *hexutil.Uint64
}) ([]*Block, error) {
	from := rpc.BlockNumber(args.From)

	var to rpc.BlockNumber
	if args.To != nil {
		to = rpc.BlockNumber(*args.To)
	} else {
		to = rpc.BlockNumber(r.backend.CurrentBlock().Number().Int64())
	}
	if to < from {
		return []*Block{}, nil
	}
	ret := make([]*Block, 0, to-from+1)
	for i := from; i <= to; i++ {
		numberOrHash := rpc.BlockNumberOrHashWithNumber(i)
		ret = append(ret, &Block{
			backend:      r.backend,
			numberOrHash: &numberOrHash,
		})
	}
	return ret, nil
}

func (r *Resolver) Pending(ctx context.Context) *Pending {
	return &Pending{r.backend}
}

func (r *Resolver) Transaction(ctx context.Context, args struct{ Hash common.Hash }) (*Transaction, error) {
	tx := &Transaction{
		backend: r.backend,
		hash:    args.Hash,
	}
	
	t, err := tx.resolve(ctx)
	if err != nil {
		return nil, err
	} else if t == nil {
		return nil, nil
	}
	return tx, nil
}

func (r *Resolver) SendRawTransaction(ctx context.Context, args struct{ Data hexutil.Bytes }) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := rlp.DecodeBytes(args.Data, tx); err != nil {
		return common.Hash{}, err
	}
	hash, err := ethapi.SubmitTransaction(ctx, r.backend, tx)
	return hash, err
}


type FilterCriteria struct {
	FromBlock *hexutil.Uint64   
	ToBlock   *hexutil.Uint64   
	Addresses *[]common.Address 

	
	
	
	
	
	
	
	
	
	
	
	Topics *[][]common.Hash
}

func (r *Resolver) Logs(ctx context.Context, args struct{ Filter FilterCriteria }) ([]*Log, error) {
	
	begin := rpc.LatestBlockNumber.Int64()
	if args.Filter.FromBlock != nil {
		begin = int64(*args.Filter.FromBlock)
	}
	end := rpc.LatestBlockNumber.Int64()
	if args.Filter.ToBlock != nil {
		end = int64(*args.Filter.ToBlock)
	}
	var addresses []common.Address
	if args.Filter.Addresses != nil {
		addresses = *args.Filter.Addresses
	}
	var topics [][]common.Hash
	if args.Filter.Topics != nil {
		topics = *args.Filter.Topics
	}
	
	filter := filters.NewRangeFilter(filters.Backend(r.backend), begin, end, addresses, topics)
	return runFilter(ctx, r.backend, filter)
}

func (r *Resolver) GasPrice(ctx context.Context) (hexutil.Big, error) {
	price, err := r.backend.SuggestPrice(ctx)
	return hexutil.Big(*price), err
}

func (r *Resolver) ProtocolVersion(ctx context.Context) (int32, error) {
	return int32(r.backend.ProtocolVersion()), nil
}

func (r *Resolver) ChainID(ctx context.Context) (hexutil.Big, error) {
	return hexutil.Big(*r.backend.ChainConfig().ChainID), nil
}


type SyncState struct {
	progress ethereum.SyncProgress
}

func (s *SyncState) StartingBlock() hexutil.Uint64 {
	return hexutil.Uint64(s.progress.StartingBlock)
}

func (s *SyncState) CurrentBlock() hexutil.Uint64 {
	return hexutil.Uint64(s.progress.CurrentBlock)
}

func (s *SyncState) HighestBlock() hexutil.Uint64 {
	return hexutil.Uint64(s.progress.HighestBlock)
}

func (s *SyncState) PulledStates() *hexutil.Uint64 {
	ret := hexutil.Uint64(s.progress.PulledStates)
	return &ret
}

func (s *SyncState) KnownStates() *hexutil.Uint64 {
	ret := hexutil.Uint64(s.progress.KnownStates)
	return &ret
}








func (r *Resolver) Syncing() (*SyncState, error) {
	progress := r.backend.Downloader().Progress()

	
	if progress.CurrentBlock >= progress.HighestBlock {
		return nil, nil
	}
	
	return &SyncState{progress}, nil
}
