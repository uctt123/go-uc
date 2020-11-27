















package core

import (
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)




type Validator interface {
	
	ValidateBody(block *types.Block) error

	
	
	ValidateState(block *types.Block, state *state.StateDB, receipts types.Receipts, usedGas uint64) error
}


type Prefetcher interface {
	
	
	
	Prefetch(block *types.Block, statedb *state.StateDB, cfg vm.Config, interrupt *uint32)
}


type Processor interface {
	
	
	
	Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error)
}
