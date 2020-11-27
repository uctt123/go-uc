















package core

import (
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)




type statePrefetcher struct {
	config *params.ChainConfig 
	bc     *BlockChain         
	engine consensus.Engine    
}


func newStatePrefetcher(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *statePrefetcher {
	return &statePrefetcher{
		config: config,
		bc:     bc,
		engine: engine,
	}
}




func (p *statePrefetcher) Prefetch(block *types.Block, statedb *state.StateDB, cfg vm.Config, interrupt *uint32) {
	var (
		header  = block.Header()
		gaspool = new(GasPool).AddGas(block.GasLimit())
	)
	
	byzantium := p.config.IsByzantium(block.Number())
	for i, tx := range block.Transactions() {
		
		if interrupt != nil && atomic.LoadUint32(interrupt) == 1 {
			return
		}
		
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		if err := precacheTransaction(p.config, p.bc, nil, gaspool, statedb, header, tx, cfg); err != nil {
			return 
		}
		
		if !byzantium {
			statedb.IntermediateRoot(true)
		}
	}
	
	if byzantium {
		statedb.IntermediateRoot(true)
	}
}




func precacheTransaction(config *params.ChainConfig, bc ChainContext, author *common.Address, gaspool *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, cfg vm.Config) error {
	
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return err
	}
	
	context := NewEVMContext(msg, header, bc, author)
	vm := vm.NewEVM(context, statedb, config, cfg)

	_, err = ApplyMessage(vm, msg, gaspool)
	return err
}
