















package abi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)



type Argument struct {
	Name    string
	Type    Type
	Indexed bool 
}

type Arguments []Argument

type ArgumentMarshaling struct {
	Name         string
	Type         string
	InternalType string
	Components   []ArgumentMarshaling
	Indexed      bool
}


func (argument *Argument) UnmarshalJSON(data []byte) error {
	var arg ArgumentMarshaling
	err := json.Unmarshal(data, &arg)
	if err != nil {
		return fmt.Errorf("argument json err: %v", err)
	}

	argument.Type, err = NewType(arg.Type, arg.InternalType, arg.Components)
	if err != nil {
		return err
	}
	argument.Name = arg.Name
	argument.Indexed = arg.Indexed

	return nil
}


func (arguments Arguments) NonIndexed() Arguments {
	var ret []Argument
	for _, arg := range arguments {
		if !arg.Indexed {
			ret = append(ret, arg)
		}
	}
	return ret
}


func (arguments Arguments) isTuple() bool {
	return len(arguments) > 1
}


func (arguments Arguments) Unpack(data []byte) ([]interface{}, error) {
	if len(data) == 0 {
		if len(arguments) != 0 {
			return nil, fmt.Errorf("abi: attempting to unmarshall an empty string while arguments are expected")
		}
		
		nonIndexedArgs := arguments.NonIndexed()
		defaultVars := make([]interface{}, len(nonIndexedArgs))
		for index, arg := range nonIndexedArgs {
			defaultVars[index] = reflect.New(arg.Type.GetType())
		}
		return defaultVars, nil
	}
	return arguments.UnpackValues(data)
}


func (arguments Arguments) UnpackIntoMap(v map[string]interface{}, data []byte) error {
	
	if v == nil {
		return fmt.Errorf("abi: cannot unpack into a nil map")
	}
	if len(data) == 0 {
		if len(arguments) != 0 {
			return fmt.Errorf("abi: attempting to unmarshall an empty string while arguments are expected")
		}
		return nil 
	}
	marshalledValues, err := arguments.UnpackValues(data)
	if err != nil {
		return err
	}
	for i, arg := range arguments.NonIndexed() {
		v[arg.Name] = marshalledValues[i]
	}
	return nil
}


func (arguments Arguments) Copy(v interface{}, values []interface{}) error {
	
	if reflect.Ptr != reflect.ValueOf(v).Kind() {
		return fmt.Errorf("abi: Unpack(non-pointer %T)", v)
	}
	if len(values) == 0 {
		if len(arguments) != 0 {
			return fmt.Errorf("abi: attempting to copy no values while %d arguments are expected", len(arguments))
		}
		return nil 
	}
	if arguments.isTuple() {
		return arguments.copyTuple(v, values)
	}
	return arguments.copyAtomic(v, values[0])
}


func (arguments Arguments) copyAtomic(v interface{}, marshalledValues interface{}) error {
	dst := reflect.ValueOf(v).Elem()
	src := reflect.ValueOf(marshalledValues)

	if dst.Kind() == reflect.Struct && src.Kind() != reflect.Struct {
		return set(dst.Field(0), src)
	}
	return set(dst, src)
}


func (arguments Arguments) copyTuple(v interface{}, marshalledValues []interface{}) error {
	value := reflect.ValueOf(v).Elem()
	nonIndexedArgs := arguments.NonIndexed()

	switch value.Kind() {
	case reflect.Struct:
		argNames := make([]string, len(nonIndexedArgs))
		for i, arg := range nonIndexedArgs {
			argNames[i] = arg.Name
		}
		var err error
		abi2struct, err := mapArgNamesToStructFields(argNames, value)
		if err != nil {
			return err
		}
		for i, arg := range nonIndexedArgs {
			field := value.FieldByName(abi2struct[arg.Name])
			if !field.IsValid() {
				return fmt.Errorf("abi: field %s can't be found in the given value", arg.Name)
			}
			if err := set(field, reflect.ValueOf(marshalledValues[i])); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if value.Len() < len(marshalledValues) {
			return fmt.Errorf("abi: insufficient number of arguments for unpack, want %d, got %d", len(arguments), value.Len())
		}
		for i := range nonIndexedArgs {
			if err := set(value.Index(i), reflect.ValueOf(marshalledValues[i])); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("abi:[2] cannot unmarshal tuple in to %v", value.Type())
	}
	return nil
}




func (arguments Arguments) UnpackValues(data []byte) ([]interface{}, error) {
	nonIndexedArgs := arguments.NonIndexed()
	retval := make([]interface{}, 0, len(nonIndexedArgs))
	virtualArgs := 0
	for index, arg := range nonIndexedArgs {
		marshalledValue, err := toGoType((index+virtualArgs)*32, arg.Type, data)
		if arg.Type.T == ArrayTy && !isDynamicType(arg.Type) {
			
			
			
			
			
			
			
			
			
			
			virtualArgs += getTypeSize(arg.Type)/32 - 1
		} else if arg.Type.T == TupleTy && !isDynamicType(arg.Type) {
			
			
			virtualArgs += getTypeSize(arg.Type)/32 - 1
		}
		if err != nil {
			return nil, err
		}
		retval = append(retval, marshalledValue)
	}
	return retval, nil
}



func (arguments Arguments) PackValues(args []interface{}) ([]byte, error) {
	return arguments.Pack(args...)
}


func (arguments Arguments) Pack(args ...interface{}) ([]byte, error) {
	
	abiArgs := arguments
	if len(args) != len(abiArgs) {
		return nil, fmt.Errorf("argument count mismatch: got %d for %d", len(args), len(abiArgs))
	}
	
	
	var variableInput []byte

	
	inputOffset := 0
	for _, abiArg := range abiArgs {
		inputOffset += getTypeSize(abiArg.Type)
	}
	var ret []byte
	for i, a := range args {
		input := abiArgs[i]
		
		packed, err := input.Type.pack(reflect.ValueOf(a))
		if err != nil {
			return nil, err
		}
		
		if isDynamicType(input.Type) {
			
			ret = append(ret, packNum(reflect.ValueOf(inputOffset))...)
			
			inputOffset += len(packed)
			
			variableInput = append(variableInput, packed...)
		} else {
			
			ret = append(ret, packed...)
		}
	}
	
	ret = append(ret, variableInput...)

	return ret, nil
}


func ToCamelCase(input string) string {
	parts := strings.Split(input, "_")
	for i, s := range parts {
		if len(s) > 0 {
			parts[i] = strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return strings.Join(parts, "")
}
