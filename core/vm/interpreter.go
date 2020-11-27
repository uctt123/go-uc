















package vm

import (
	"hash"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/log"
)


type Config struct {
	Debug                   bool   
	Tracer                  Tracer 
	NoRecursion             bool   
	EnablePreimageRecording bool   

	JumpTable [256]*operation 

	EWASMInterpreter string 
	EVMInterpreter   string 

	ExtraEips []int 
}





type Interpreter interface {
	
	
	Run(contract *Contract, input []byte, static bool) ([]byte, error)
	
	
	
	
	
	
	
	
	
	
	
	CanRun([]byte) bool
}



type callCtx struct {
	memory   *Memory
	stack    *Stack
	rstack   *ReturnStack
	contract *Contract
}




type keccakState interface {
	hash.Hash
	Read([]byte) (int, error)
}


type EVMInterpreter struct {
	evm *EVM
	cfg Config

	hasher    keccakState 
	hasherBuf common.Hash 

	readOnly   bool   
	returnData []byte 
}


func NewEVMInterpreter(evm *EVM, cfg Config) *EVMInterpreter {
	
	
	
	if cfg.JumpTable[STOP] == nil {
		var jt JumpTable
		switch {
		case evm.chainRules.IsYoloV1:
			jt = yoloV1InstructionSet
		case evm.chainRules.IsIstanbul:
			jt = istanbulInstructionSet
		case evm.chainRules.IsConstantinople:
			jt = constantinopleInstructionSet
		case evm.chainRules.IsByzantium:
			jt = byzantiumInstructionSet
		case evm.chainRules.IsEIP158:
			jt = spuriousDragonInstructionSet
		case evm.chainRules.IsEIP150:
			jt = tangerineWhistleInstructionSet
		case evm.chainRules.IsHomestead:
			jt = homesteadInstructionSet
		default:
			jt = frontierInstructionSet
		}
		for i, eip := range cfg.ExtraEips {
			if err := EnableEIP(eip, &jt); err != nil {
				
				cfg.ExtraEips = append(cfg.ExtraEips[:i], cfg.ExtraEips[i+1:]...)
				log.Error("EIP activation failed", "eip", eip, "error", err)
			}
		}
		cfg.JumpTable = jt
	}

	return &EVMInterpreter{
		evm: evm,
		cfg: cfg,
	}
}







func (in *EVMInterpreter) Run(contract *Contract, input []byte, readOnly bool) (ret []byte, err error) {

	
	in.evm.depth++
	defer func() { in.evm.depth-- }()

	
	
	if readOnly && !in.readOnly {
		in.readOnly = true
		defer func() { in.readOnly = false }()
	}

	
	
	in.returnData = nil

	
	if len(contract.Code) == 0 {
		return nil, nil
	}

	var (
		op          OpCode             
		mem         = NewMemory()      
		stack       = newstack()       
		returns     = newReturnStack() 
		callContext = &callCtx{
			memory:   mem,
			stack:    stack,
			rstack:   returns,
			contract: contract,
		}
		
		
		
		pc   = uint64(0) 
		cost uint64
		
		pcCopy  uint64 
		gasCopy uint64 
		logged  bool   
		res     []byte 
	)
	
	
	
	defer func() {
		returnStack(stack)
		returnRStack(returns)
	}()
	contract.Input = input

	if in.cfg.Debug {
		defer func() {
			if err != nil {
				if !logged {
					in.cfg.Tracer.CaptureState(in.evm, pcCopy, op, gasCopy, cost, mem, stack, returns, in.returnData, contract, in.evm.depth, err)
				} else {
					in.cfg.Tracer.CaptureFault(in.evm, pcCopy, op, gasCopy, cost, mem, stack, returns, contract, in.evm.depth, err)
				}
			}
		}()
	}
	
	
	
	
	steps := 0
	for {
		steps++
		if steps%1000 == 0 && atomic.LoadInt32(&in.evm.abort) != 0 {
			break
		}
		if in.cfg.Debug {
			
			logged, pcCopy, gasCopy = false, pc, contract.Gas
		}

		
		
		op = contract.GetOp(pc)
		operation := in.cfg.JumpTable[op]
		if operation == nil {
			return nil, &ErrInvalidOpCode{opcode: op}
		}
		
		if sLen := stack.len(); sLen < operation.minStack {
			return nil, &ErrStackUnderflow{stackLen: sLen, required: operation.minStack}
		} else if sLen > operation.maxStack {
			return nil, &ErrStackOverflow{stackLen: sLen, limit: operation.maxStack}
		}
		
		if in.readOnly && in.evm.chainRules.IsByzantium {
			
			
			
			
			
			if operation.writes || (op == CALL && stack.Back(2).Sign() != 0) {
				return nil, ErrWriteProtection
			}
		}
		
		cost = operation.constantGas 
		if !contract.UseGas(operation.constantGas) {
			return nil, ErrOutOfGas
		}

		var memorySize uint64
		
		
		
		
		if operation.memorySize != nil {
			memSize, overflow := operation.memorySize(stack)
			if overflow {
				return nil, ErrGasUintOverflow
			}
			
			
			if memorySize, overflow = math.SafeMul(toWordSize(memSize), 32); overflow {
				return nil, ErrGasUintOverflow
			}
		}
		
		
		
		if operation.dynamicGas != nil {
			var dynamicCost uint64
			dynamicCost, err = operation.dynamicGas(in.evm, contract, stack, mem, memorySize)
			cost += dynamicCost 
			if err != nil || !contract.UseGas(dynamicCost) {
				return nil, ErrOutOfGas
			}
		}
		if memorySize > 0 {
			mem.Resize(memorySize)
		}

		if in.cfg.Debug {
			in.cfg.Tracer.CaptureState(in.evm, pc, op, gasCopy, cost, mem, stack, returns, in.returnData, contract, in.evm.depth, err)
			logged = true
		}

		
		res, err = operation.execute(&pc, in, callContext)
		
		
		if operation.returns {
			in.returnData = common.CopyBytes(res)
		}

		switch {
		case err != nil:
			return nil, err
		case operation.reverts:
			return res, ErrExecutionReverted
		case operation.halts:
			return res, nil
		case !operation.jumps:
			pc++
		}
	}
	return nil, nil
}



func (in *EVMInterpreter) CanRun(code []byte) bool {
	return true
}
