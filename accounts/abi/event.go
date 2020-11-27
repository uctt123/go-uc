















package abi

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)




type Event struct {
	
	
	
	
	
	
	
	
	
	Name string
	
	RawName   string
	Anonymous bool
	Inputs    Arguments
	str       string
	
	
	
	Sig string
	
	
	ID common.Hash
}





func NewEvent(name, rawName string, anonymous bool, inputs Arguments) Event {
	
	
	names := make([]string, len(inputs))
	types := make([]string, len(inputs))
	for i, input := range inputs {
		if input.Name == "" {
			inputs[i] = Argument{
				Name:    fmt.Sprintf("arg%d", i),
				Indexed: input.Indexed,
				Type:    input.Type,
			}
		} else {
			inputs[i] = input
		}
		
		names[i] = fmt.Sprintf("%v %v", input.Type, inputs[i].Name)
		if input.Indexed {
			names[i] = fmt.Sprintf("%v indexed %v", input.Type, inputs[i].Name)
		}
		
		types[i] = input.Type.String()
	}

	str := fmt.Sprintf("event %v(%v)", rawName, strings.Join(names, ", "))
	sig := fmt.Sprintf("%v(%v)", rawName, strings.Join(types, ","))
	id := common.BytesToHash(crypto.Keccak256([]byte(sig)))

	return Event{
		Name:      name,
		RawName:   rawName,
		Anonymous: anonymous,
		Inputs:    inputs,
		str:       str,
		Sig:       sig,
		ID:        id,
	}
}

func (e Event) String() string {
	return e.str
}
