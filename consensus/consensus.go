
















package consensus

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)



type ChainHeaderReader interface {
	
	Config() *params.ChainConfig

	
	CurrentHeader() *types.Header

	
	GetHeader(hash common.Hash, number uint64) *types.Header

	
	GetHeaderByNumber(number uint64) *types.Header

	
	GetHeaderByHash(hash common.Hash) *types.Header
}



type ChainReader interface {
	ChainHeaderReader

	
	GetBlock(hash common.Hash, number uint64) *types.Block
}


type Engine interface {
	
	
	
	Author(header *types.Header) (common.Address, error)

	
	
	
	VerifyHeader(chain ChainHeaderReader, header *types.Header, seal bool) error

	
	
	
	
	VerifyHeaders(chain ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

	
	
	VerifyUncles(chain ChainReader, block *types.Block) error

	
	
	VerifySeal(chain ChainHeaderReader, header *types.Header) error

	
	
	Prepare(chain ChainHeaderReader, header *types.Header) error

	
	
	
	
	
	Finalize(chain ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
		uncles []*types.Header)

	
	
	
	
	
	FinalizeAndAssemble(chain ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
		uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error)

	
	
	
	
	
	Seal(chain ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error

	
	SealHash(header *types.Header) common.Hash

	
	
	CalcDifficulty(chain ChainHeaderReader, time uint64, parent *types.Header) *big.Int

	
	APIs(chain ChainHeaderReader) []rpc.API

	
	Close() error
}


type PoW interface {
	Engine

	
	Hashrate() float64
}
