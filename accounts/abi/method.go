















package abi

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)


type FunctionType int

const (
	
	
	Constructor FunctionType = iota
	
	
	
	Fallback
	
	
	Receive
	
	Function
)









type Method struct {
	
	
	
	
	
	
	
	
	
	Name    string
	RawName string 

	
	
	Type FunctionType

	
	
	
	StateMutability string

	
	Constant bool
	Payable  bool

	Inputs  Arguments
	Outputs Arguments
	str     string
	
	
	
	Sig string
	
	
	ID []byte
}





func NewMethod(name string, rawName string, funType FunctionType, mutability string, isConst, isPayable bool, inputs Arguments, outputs Arguments) Method {
	var (
		types       = make([]string, len(inputs))
		inputNames  = make([]string, len(inputs))
		outputNames = make([]string, len(outputs))
	)
	for i, input := range inputs {
		inputNames[i] = fmt.Sprintf("%v %v", input.Type, input.Name)
		types[i] = input.Type.String()
	}
	for i, output := range outputs {
		outputNames[i] = output.Type.String()
		if len(output.Name) > 0 {
			outputNames[i] += fmt.Sprintf(" %v", output.Name)
		}
	}
	
	
	var (
		sig string
		id  []byte
	)
	if funType == Function {
		sig = fmt.Sprintf("%v(%v)", rawName, strings.Join(types, ","))
		id = crypto.Keccak256([]byte(sig))[:4]
	}
	
	
	state := mutability
	if state == "nonpayable" {
		state = ""
	}
	if state != "" {
		state = state + " "
	}
	identity := fmt.Sprintf("function %v", rawName)
	if funType == Fallback {
		identity = "fallback"
	} else if funType == Receive {
		identity = "receive"
	} else if funType == Constructor {
		identity = "constructor"
	}
	str := fmt.Sprintf("%v(%v) %sreturns(%v)", identity, strings.Join(inputNames, ", "), state, strings.Join(outputNames, ", "))

	return Method{
		Name:            name,
		RawName:         rawName,
		Type:            funType,
		StateMutability: mutability,
		Constant:        isConst,
		Payable:         isPayable,
		Inputs:          inputs,
		Outputs:         outputs,
		str:             str,
		Sig:             sig,
		ID:              id,
	}
}

func (method Method) String() string {
	return method.str
}


func (method Method) IsConstant() bool {
	return method.StateMutability == "view" || method.StateMutability == "pure" || method.Constant
}



func (method Method) IsPayable() bool {
	return method.StateMutability == "payable" || method.Payable
}
