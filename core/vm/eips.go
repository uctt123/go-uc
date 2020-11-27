















package vm

import (
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

var activators = map[int]func(*JumpTable){
	2200: enable2200,
	1884: enable1884,
	1344: enable1344,
	2315: enable2315,
}




func EnableEIP(eipNum int, jt *JumpTable) error {
	enablerFn, ok := activators[eipNum]
	if !ok {
		return fmt.Errorf("undefined eip %d", eipNum)
	}
	enablerFn(jt)
	return nil
}

func ValidEip(eipNum int) bool {
	_, ok := activators[eipNum]
	return ok
}
func ActivateableEips() []string {
	var nums []string
	for k := range activators {
		nums = append(nums, fmt.Sprintf("%d", k))
	}
	sort.Strings(nums)
	return nums
}






func enable1884(jt *JumpTable) {
	
	jt[SLOAD].constantGas = params.SloadGasEIP1884
	jt[BALANCE].constantGas = params.BalanceGasEIP1884
	jt[EXTCODEHASH].constantGas = params.ExtcodeHashGasEIP1884

	
	jt[SELFBALANCE] = &operation{
		execute:     opSelfBalance,
		constantGas: GasFastStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
	}
}

func opSelfBalance(pc *uint64, interpreter *EVMInterpreter, callContext *callCtx) ([]byte, error) {
	balance, _ := uint256.FromBig(interpreter.evm.StateDB.GetBalance(callContext.contract.Address()))
	callContext.stack.push(balance)
	return nil, nil
}



func enable1344(jt *JumpTable) {
	
	jt[CHAINID] = &operation{
		execute:     opChainID,
		constantGas: GasQuickStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
	}
}


func opChainID(pc *uint64, interpreter *EVMInterpreter, callContext *callCtx) ([]byte, error) {
	chainId, _ := uint256.FromBig(interpreter.evm.chainConfig.ChainID)
	callContext.stack.push(chainId)
	return nil, nil
}


func enable2200(jt *JumpTable) {
	jt[SLOAD].constantGas = params.SloadGasEIP2200
	jt[SSTORE].dynamicGas = gasSStoreEIP2200
}



func enable2315(jt *JumpTable) {
	
	jt[BEGINSUB] = &operation{
		execute:     opBeginSub,
		constantGas: GasQuickStep,
		minStack:    minStack(0, 0),
		maxStack:    maxStack(0, 0),
	}
	
	jt[JUMPSUB] = &operation{
		execute:     opJumpSub,
		constantGas: GasSlowStep,
		minStack:    minStack(1, 0),
		maxStack:    maxStack(1, 0),
		jumps:       true,
	}
	
	jt[RETURNSUB] = &operation{
		execute:     opReturnSub,
		constantGas: GasFastStep,
		minStack:    minStack(0, 0),
		maxStack:    maxStack(0, 0),
		jumps:       true,
	}
}
