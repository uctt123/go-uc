















package t8ntool

import (
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"golang.org/x/crypto/sha3"
)

type Prestate struct {
	Env stEnv             `json:"env"`
	Pre core.GenesisAlloc `json:"pre"`
}



type ExecutionResult struct {
	StateRoot   common.Hash    `json:"stateRoot"`
	TxRoot      common.Hash    `json:"txRoot"`
	ReceiptRoot common.Hash    `json:"receiptRoot"`
	LogsHash    common.Hash    `json:"logsHash"`
	Bloom       types.Bloom    `json:"logsBloom"        gencodec:"required"`
	Receipts    types.Receipts `json:"receipts"`
	Rejected    []int          `json:"rejected,omitempty"`
}

type ommer struct {
	Delta   uint64         `json:"delta"`
	Address common.Address `json:"address"`
}


type stEnv struct {
	Coinbase    common.Address                      `json:"currentCoinbase"   gencodec:"required"`
	Difficulty  *big.Int                            `json:"currentDifficulty" gencodec:"required"`
	GasLimit    uint64                              `json:"currentGasLimit"   gencodec:"required"`
	Number      uint64                              `json:"currentNumber"     gencodec:"required"`
	Timestamp   uint64                              `json:"currentTimestamp"  gencodec:"required"`
	BlockHashes map[math.HexOrDecimal64]common.Hash `json:"blockHashes,omitempty"`
	Ommers      []ommer                             `json:"ommers,omitempty"`
}

type stEnvMarshaling struct {
	Coinbase   common.UnprefixedAddress
	Difficulty *math.HexOrDecimal256
	GasLimit   math.HexOrDecimal64
	Number     math.HexOrDecimal64
	Timestamp  math.HexOrDecimal64
}


func (pre *Prestate) Apply(vmConfig vm.Config, chainConfig *params.ChainConfig,
	txs types.Transactions, miningReward int64,
	getTracerFn func(txIndex int, txHash common.Hash) (tracer vm.Tracer, err error)) (*state.StateDB, *ExecutionResult, error) {

	
	
	var hashError error
	getHash := func(num uint64) common.Hash {
		if pre.Env.BlockHashes == nil {
			hashError = fmt.Errorf("getHash(%d) invoked, no blockhashes provided", num)
			return common.Hash{}
		}
		h, ok := pre.Env.BlockHashes[math.HexOrDecimal64(num)]
		if !ok {
			hashError = fmt.Errorf("getHash(%d) invoked, blockhash for that block not provided", num)
		}
		return h
	}
	var (
		statedb     = MakePreState(rawdb.NewMemoryDatabase(), pre.Pre)
		signer      = types.MakeSigner(chainConfig, new(big.Int).SetUint64(pre.Env.Number))
		gaspool     = new(core.GasPool)
		blockHash   = common.Hash{0x13, 0x37}
		rejectedTxs []int
		includedTxs types.Transactions
		gasUsed     = uint64(0)
		receipts    = make(types.Receipts, 0)
		txIndex     = 0
	)
	gaspool.AddGas(pre.Env.GasLimit)
	vmContext := vm.Context{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		Coinbase:    pre.Env.Coinbase,
		BlockNumber: new(big.Int).SetUint64(pre.Env.Number),
		Time:        new(big.Int).SetUint64(pre.Env.Timestamp),
		Difficulty:  pre.Env.Difficulty,
		GasLimit:    pre.Env.GasLimit,
		GetHash:     getHash,
		
	}
	
	
	if chainConfig.DAOForkSupport &&
		chainConfig.DAOForkBlock != nil &&
		chainConfig.DAOForkBlock.Cmp(new(big.Int).SetUint64(pre.Env.Number)) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}

	for i, tx := range txs {
		msg, err := tx.AsMessage(signer)
		if err != nil {
			log.Info("rejected tx", "index", i, "hash", tx.Hash(), "error", err)
			rejectedTxs = append(rejectedTxs, i)
			continue
		}
		tracer, err := getTracerFn(txIndex, tx.Hash())
		if err != nil {
			return nil, nil, err
		}
		vmConfig.Tracer = tracer
		vmConfig.Debug = (tracer != nil)
		statedb.Prepare(tx.Hash(), blockHash, txIndex)
		vmContext.GasPrice = msg.GasPrice()
		vmContext.Origin = msg.From()

		evm := vm.NewEVM(vmContext, statedb, chainConfig, vmConfig)
		snapshot := statedb.Snapshot()
		
		msgResult, err := core.ApplyMessage(evm, msg, gaspool)
		if err != nil {
			statedb.RevertToSnapshot(snapshot)
			log.Info("rejected tx", "index", i, "hash", tx.Hash(), "from", msg.From(), "error", err)
			rejectedTxs = append(rejectedTxs, i)
			continue
		}
		includedTxs = append(includedTxs, tx)
		if hashError != nil {
			return nil, nil, NewError(ErrorMissingBlockhash, hashError)
		}
		gasUsed += msgResult.UsedGas
		
		{
			var root []byte
			if chainConfig.IsByzantium(vmContext.BlockNumber) {
				statedb.Finalise(true)
			} else {
				root = statedb.IntermediateRoot(chainConfig.IsEIP158(vmContext.BlockNumber)).Bytes()
			}

			receipt := types.NewReceipt(root, msgResult.Failed(), gasUsed)
			receipt.TxHash = tx.Hash()
			receipt.GasUsed = msgResult.UsedGas
			
			if msg.To() == nil {
				receipt.ContractAddress = crypto.CreateAddress(evm.Context.Origin, tx.Nonce())
			}
			
			receipt.Logs = statedb.GetLogs(tx.Hash())
			receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
			
			
			
			receipt.TransactionIndex = uint(txIndex)
			receipts = append(receipts, receipt)
		}
		txIndex++
	}
	statedb.IntermediateRoot(chainConfig.IsEIP158(vmContext.BlockNumber))
	
	if miningReward > 0 {
		
		
		
		
		
		var (
			blockReward = big.NewInt(miningReward)
			minerReward = new(big.Int).Set(blockReward)
			perOmmer    = new(big.Int).Div(blockReward, big.NewInt(32))
		)
		for _, ommer := range pre.Env.Ommers {
			
			minerReward.Add(minerReward, perOmmer)
			
			reward := big.NewInt(8)
			reward.Sub(reward, big.NewInt(0).SetUint64(ommer.Delta))
			reward.Mul(reward, blockReward)
			reward.Div(reward, big.NewInt(8))
			statedb.AddBalance(ommer.Address, reward)
		}
		statedb.AddBalance(pre.Env.Coinbase, minerReward)
	}
	
	root, err := statedb.Commit(chainConfig.IsEIP158(vmContext.BlockNumber))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not commit state: %v", err)
		return nil, nil, NewError(ErrorEVM, fmt.Errorf("could not commit state: %v", err))
	}
	execRs := &ExecutionResult{
		StateRoot:   root,
		TxRoot:      types.DeriveSha(includedTxs, new(trie.Trie)),
		ReceiptRoot: types.DeriveSha(receipts, new(trie.Trie)),
		Bloom:       types.CreateBloom(receipts),
		LogsHash:    rlpHash(statedb.Logs()),
		Receipts:    receipts,
		Rejected:    rejectedTxs,
	}
	return statedb, execRs, nil
}

func MakePreState(db ethdb.Database, accounts core.GenesisAlloc) *state.StateDB {
	sdb := state.NewDatabase(db)
	statedb, _ := state.New(common.Hash{}, sdb, nil)
	for addr, a := range accounts {
		statedb.SetCode(addr, a.Code)
		statedb.SetNonce(addr, a.Nonce)
		statedb.SetBalance(addr, a.Balance)
		for k, v := range a.Storage {
			statedb.SetState(addr, k, v)
		}
	}
	
	root, _ := statedb.Commit(false)
	statedb, _ = state.New(root, sdb, nil)
	return statedb
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
