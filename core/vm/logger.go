















package vm

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
)

var errTraceLimitReached = errors.New("the number of logs reached the specified limit")


type Storage map[common.Hash]common.Hash


func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}
	return cpy
}


type LogConfig struct {
	DisableMemory     bool 
	DisableStack      bool 
	DisableStorage    bool 
	DisableReturnData bool 
	Debug             bool 
	Limit             int  
}





type StructLog struct {
	Pc            uint64                      `json:"pc"`
	Op            OpCode                      `json:"op"`
	Gas           uint64                      `json:"gas"`
	GasCost       uint64                      `json:"gasCost"`
	Memory        []byte                      `json:"memory"`
	MemorySize    int                         `json:"memSize"`
	Stack         []*big.Int                  `json:"stack"`
	ReturnStack   []uint32                    `json:"returnStack"`
	ReturnData    []byte                      `json:"returnData"`
	Storage       map[common.Hash]common.Hash `json:"-"`
	Depth         int                         `json:"depth"`
	RefundCounter uint64                      `json:"refund"`
	Err           error                       `json:"-"`
}


type structLogMarshaling struct {
	Stack       []*math.HexOrDecimal256
	ReturnStack []math.HexOrDecimal64
	Gas         math.HexOrDecimal64
	GasCost     math.HexOrDecimal64
	Memory      hexutil.Bytes
	OpName      string `json:"opName"` 
	ErrorString string `json:"error"`  
}


func (s *StructLog) OpName() string {
	return s.Op.String()
}


func (s *StructLog) ErrorString() string {
	if s.Err != nil {
		return s.Err.Error()
	}
	return ""
}






type Tracer interface {
	CaptureStart(from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) error
	CaptureState(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, rStack *ReturnStack, rData []byte, contract *Contract, depth int, err error) error
	CaptureFault(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, rStack *ReturnStack, contract *Contract, depth int, err error) error
	CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) error
}






type StructLogger struct {
	cfg LogConfig

	storage map[common.Address]Storage
	logs    []StructLog
	output  []byte
	err     error
}


func NewStructLogger(cfg *LogConfig) *StructLogger {
	logger := &StructLogger{
		storage: make(map[common.Address]Storage),
	}
	if cfg != nil {
		logger.cfg = *cfg
	}
	return logger
}


func (l *StructLogger) CaptureStart(from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) error {
	return nil
}




func (l *StructLogger) CaptureState(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, rStack *ReturnStack, rData []byte, contract *Contract, depth int, err error) error {
	
	if l.cfg.Limit != 0 && l.cfg.Limit <= len(l.logs) {
		return errTraceLimitReached
	}
	
	var mem []byte
	if !l.cfg.DisableMemory {
		mem = make([]byte, len(memory.Data()))
		copy(mem, memory.Data())
	}
	
	var stck []*big.Int
	if !l.cfg.DisableStack {
		stck = make([]*big.Int, len(stack.Data()))
		for i, item := range stack.Data() {
			stck[i] = new(big.Int).Set(item.ToBig())
		}
	}
	var rstack []uint32
	if !l.cfg.DisableStack && rStack != nil {
		rstck := make([]uint32, len(rStack.data))
		copy(rstck, rStack.data)
	}
	
	var storage Storage
	if !l.cfg.DisableStorage {
		
		
		if l.storage[contract.Address()] == nil {
			l.storage[contract.Address()] = make(Storage)
		}
		
		if op == SLOAD && stack.len() >= 1 {
			var (
				address = common.Hash(stack.data[stack.len()-1].Bytes32())
				value   = env.StateDB.GetState(contract.Address(), address)
			)
			l.storage[contract.Address()][address] = value
		}
		
		if op == SSTORE && stack.len() >= 2 {
			var (
				value   = common.Hash(stack.data[stack.len()-2].Bytes32())
				address = common.Hash(stack.data[stack.len()-1].Bytes32())
			)
			l.storage[contract.Address()][address] = value
		}
		storage = l.storage[contract.Address()].Copy()
	}
	var rdata []byte
	if !l.cfg.DisableReturnData {
		rdata = make([]byte, len(rData))
		copy(rdata, rData)
	}
	
	log := StructLog{pc, op, gas, cost, mem, memory.Len(), stck, rstack, rdata, storage, depth, env.StateDB.GetRefund(), err}
	l.logs = append(l.logs, log)
	return nil
}



func (l *StructLogger) CaptureFault(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, rStack *ReturnStack, contract *Contract, depth int, err error) error {
	return nil
}


func (l *StructLogger) CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) error {
	l.output = output
	l.err = err
	if l.cfg.Debug {
		fmt.Printf("0x%x\n", output)
		if err != nil {
			fmt.Printf(" error: %v\n", err)
		}
	}
	return nil
}


func (l *StructLogger) StructLogs() []StructLog { return l.logs }


func (l *StructLogger) Error() error { return l.err }


func (l *StructLogger) Output() []byte { return l.output }


func WriteTrace(writer io.Writer, logs []StructLog) {
	for _, log := range logs {
		fmt.Fprintf(writer, "%-16spc=%08d gas=%v cost=%v", log.Op, log.Pc, log.Gas, log.GasCost)
		if log.Err != nil {
			fmt.Fprintf(writer, " ERROR: %v", log.Err)
		}
		fmt.Fprintln(writer)

		if len(log.Stack) > 0 {
			fmt.Fprintln(writer, "Stack:")
			for i := len(log.Stack) - 1; i >= 0; i-- {
				fmt.Fprintf(writer, "%08d  %x\n", len(log.Stack)-i-1, math.PaddedBigBytes(log.Stack[i], 32))
			}
		}
		if len(log.ReturnStack) > 0 {
			fmt.Fprintln(writer, "ReturnStack:")
			for i := len(log.Stack) - 1; i >= 0; i-- {
				fmt.Fprintf(writer, "%08d  0x%x (%d)\n", len(log.Stack)-i-1, log.ReturnStack[i], log.ReturnStack[i])
			}
		}
		if len(log.Memory) > 0 {
			fmt.Fprintln(writer, "Memory:")
			fmt.Fprint(writer, hex.Dump(log.Memory))
		}
		if len(log.Storage) > 0 {
			fmt.Fprintln(writer, "Storage:")
			for h, item := range log.Storage {
				fmt.Fprintf(writer, "%x: %x\n", h, item)
			}
		}
		if len(log.ReturnData) > 0 {
			fmt.Fprintln(writer, "ReturnData:")
			fmt.Fprint(writer, hex.Dump(log.ReturnData))
		}
		fmt.Fprintln(writer)
	}
}


func WriteLogs(writer io.Writer, logs []*types.Log) {
	for _, log := range logs {
		fmt.Fprintf(writer, "LOG%d: %x bn=%d txi=%x\n", len(log.Topics), log.Address, log.BlockNumber, log.TxIndex)

		for i, topic := range log.Topics {
			fmt.Fprintf(writer, "%08d  %x\n", i, topic)
		}

		fmt.Fprint(writer, hex.Dump(log.Data))
		fmt.Fprintln(writer)
	}
}

type mdLogger struct {
	out io.Writer
	cfg *LogConfig
}



func NewMarkdownLogger(cfg *LogConfig, writer io.Writer) *mdLogger {
	l := &mdLogger{writer, cfg}
	if l.cfg == nil {
		l.cfg = &LogConfig{}
	}
	return l
}

func (t *mdLogger) CaptureStart(from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) error {
	if !create {
		fmt.Fprintf(t.out, "From: `%v`\nTo: `%v`\nData: `0x%x`\nGas: `%d`\nValue `%v` wei\n",
			from.String(), to.String(),
			input, gas, value)
	} else {
		fmt.Fprintf(t.out, "From: `%v`\nCreate at: `%v`\nData: `0x%x`\nGas: `%d`\nValue `%v` wei\n",
			from.String(), to.String(),
			input, gas, value)
	}

	fmt.Fprintf(t.out, `
|  Pc   |      Op     | Cost |   Stack   |   RStack  |
|-------|-------------|------|-----------|-----------|
`)
	return nil
}

func (t *mdLogger) CaptureState(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, rStack *ReturnStack, rData []byte, contract *Contract, depth int, err error) error {
	fmt.Fprintf(t.out, "| %4d  | %10v  |  %3d |", pc, op, cost)

	if !t.cfg.DisableStack {
		
		var a []string
		for _, elem := range stack.data {
			a = append(a, fmt.Sprintf("%d", elem))
		}
		b := fmt.Sprintf("[%v]", strings.Join(a, ","))
		fmt.Fprintf(t.out, "%10v |", b)

		
		a = a[:0]
		for _, elem := range rStack.data {
			a = append(a, fmt.Sprintf("%2d", elem))
		}
		b = fmt.Sprintf("[%v]", strings.Join(a, ","))
		fmt.Fprintf(t.out, "%10v |", b)
	}
	fmt.Fprintln(t.out, "")
	if err != nil {
		fmt.Fprintf(t.out, "Error: %v\n", err)
	}
	return nil
}

func (t *mdLogger) CaptureFault(env *EVM, pc uint64, op OpCode, gas, cost uint64, memory *Memory, stack *Stack, rStack *ReturnStack, contract *Contract, depth int, err error) error {

	fmt.Fprintf(t.out, "\nError: at pc=%d, op=%v: %v\n", pc, op, err)

	return nil
}

func (t *mdLogger) CaptureEnd(output []byte, gasUsed uint64, tm time.Duration, err error) error {
	fmt.Fprintf(t.out, `
Output: 0x%x
Consumed gas: %d
Error: %v
`,
		output, gasUsed, err)
	return nil
}
