















package abi

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)


const (
	IntTy byte = iota
	UintTy
	BoolTy
	StringTy
	SliceTy
	ArrayTy
	TupleTy
	AddressTy
	FixedBytesTy
	BytesTy
	HashTy
	FixedPointTy
	FunctionTy
)


type Type struct {
	Elem *Type
	Size int
	T    byte 

	stringKind string 

	
	TupleRawName  string       
	TupleElems    []*Type      
	TupleRawNames []string     
	TupleType     reflect.Type 
}

var (
	
	typeRegex = regexp.MustCompile("([a-zA-Z]+)(([0-9]+)(x([0-9]+))?)?")
)


func NewType(t string, internalType string, components []ArgumentMarshaling) (typ Type, err error) {
	
	if strings.Count(t, "[") != strings.Count(t, "]") {
		return Type{}, fmt.Errorf("invalid arg type in abi")
	}
	typ.stringKind = t

	
	
	if strings.Count(t, "[") != 0 {
		
		subInternal := internalType
		if i := strings.LastIndex(internalType, "["); i != -1 {
			subInternal = subInternal[:i]
		}
		
		i := strings.LastIndex(t, "[")
		embeddedType, err := NewType(t[:i], subInternal, components)
		if err != nil {
			return Type{}, err
		}
		
		sliced := t[i:]
		
		re := regexp.MustCompile("[0-9]+")
		intz := re.FindAllString(sliced, -1)

		if len(intz) == 0 {
			
			typ.T = SliceTy
			typ.Elem = &embeddedType
			typ.stringKind = embeddedType.stringKind + sliced
		} else if len(intz) == 1 {
			
			typ.T = ArrayTy
			typ.Elem = &embeddedType
			typ.Size, err = strconv.Atoi(intz[0])
			if err != nil {
				return Type{}, fmt.Errorf("abi: error parsing variable size: %v", err)
			}
			typ.stringKind = embeddedType.stringKind + sliced
		} else {
			return Type{}, fmt.Errorf("invalid formatting of array type")
		}
		return typ, err
	}
	
	matches := typeRegex.FindAllStringSubmatch(t, -1)
	if len(matches) == 0 {
		return Type{}, fmt.Errorf("invalid type '%v'", t)
	}
	parsedType := matches[0]

	
	var varSize int
	if len(parsedType[3]) > 0 {
		var err error
		varSize, err = strconv.Atoi(parsedType[2])
		if err != nil {
			return Type{}, fmt.Errorf("abi: error parsing variable size: %v", err)
		}
	} else {
		if parsedType[0] == "uint" || parsedType[0] == "int" {
			
			
			return Type{}, fmt.Errorf("unsupported arg type: %s", t)
		}
	}
	
	switch varType := parsedType[1]; varType {
	case "int":
		typ.Size = varSize
		typ.T = IntTy
	case "uint":
		typ.Size = varSize
		typ.T = UintTy
	case "bool":
		typ.T = BoolTy
	case "address":
		typ.Size = 20
		typ.T = AddressTy
	case "string":
		typ.T = StringTy
	case "bytes":
		if varSize == 0 {
			typ.T = BytesTy
		} else {
			typ.T = FixedBytesTy
			typ.Size = varSize
		}
	case "tuple":
		var (
			fields     []reflect.StructField
			elems      []*Type
			names      []string
			expression string 
		)
		expression += "("
		overloadedNames := make(map[string]string)
		for idx, c := range components {
			cType, err := NewType(c.Type, c.InternalType, c.Components)
			if err != nil {
				return Type{}, err
			}
			fieldName, err := overloadedArgName(c.Name, overloadedNames)
			if err != nil {
				return Type{}, err
			}
			overloadedNames[fieldName] = fieldName
			fields = append(fields, reflect.StructField{
				Name: fieldName, 
				Type: cType.GetType(),
				Tag:  reflect.StructTag("json:\"" + c.Name + "\""),
			})
			elems = append(elems, &cType)
			names = append(names, c.Name)
			expression += cType.stringKind
			if idx != len(components)-1 {
				expression += ","
			}
		}
		expression += ")"

		typ.TupleType = reflect.StructOf(fields)
		typ.TupleElems = elems
		typ.TupleRawNames = names
		typ.T = TupleTy
		typ.stringKind = expression

		const structPrefix = "struct "
		
		
		
		if internalType != "" && strings.HasPrefix(internalType, structPrefix) {
			
			
			typ.TupleRawName = strings.Replace(internalType[len(structPrefix):], ".", "", -1)
		}

	case "function":
		typ.T = FunctionTy
		typ.Size = 24
	default:
		return Type{}, fmt.Errorf("unsupported arg type: %s", t)
	}

	return
}


func (t Type) GetType() reflect.Type {
	switch t.T {
	case IntTy:
		return reflectIntType(false, t.Size)
	case UintTy:
		return reflectIntType(true, t.Size)
	case BoolTy:
		return reflect.TypeOf(false)
	case StringTy:
		return reflect.TypeOf("")
	case SliceTy:
		return reflect.SliceOf(t.Elem.GetType())
	case ArrayTy:
		return reflect.ArrayOf(t.Size, t.Elem.GetType())
	case TupleTy:
		return t.TupleType
	case AddressTy:
		return reflect.TypeOf(common.Address{})
	case FixedBytesTy:
		return reflect.ArrayOf(t.Size, reflect.TypeOf(byte(0)))
	case BytesTy:
		return reflect.SliceOf(reflect.TypeOf(byte(0)))
	case HashTy:
		
		return reflect.ArrayOf(32, reflect.TypeOf(byte(0)))
	case FixedPointTy:
		
		return reflect.ArrayOf(32, reflect.TypeOf(byte(0)))
	case FunctionTy:
		return reflect.ArrayOf(24, reflect.TypeOf(byte(0)))
	default:
		panic("Invalid type")
	}
}

func overloadedArgName(rawName string, names map[string]string) (string, error) {
	fieldName := ToCamelCase(rawName)
	if fieldName == "" {
		return "", errors.New("abi: purely anonymous or underscored field is not supported")
	}
	
	_, ok := names[fieldName]
	for idx := 0; ok; idx++ {
		fieldName = fmt.Sprintf("%s%d", ToCamelCase(rawName), idx)
		_, ok = names[fieldName]
	}
	return fieldName, nil
}


func (t Type) String() (out string) {
	return t.stringKind
}

func (t Type) pack(v reflect.Value) ([]byte, error) {
	
	v = indirect(v)
	if err := typeCheck(t, v); err != nil {
		return nil, err
	}

	switch t.T {
	case SliceTy, ArrayTy:
		var ret []byte

		if t.requiresLengthPrefix() {
			
			ret = append(ret, packNum(reflect.ValueOf(v.Len()))...)
		}

		
		offset := 0
		offsetReq := isDynamicType(*t.Elem)
		if offsetReq {
			offset = getTypeSize(*t.Elem) * v.Len()
		}
		var tail []byte
		for i := 0; i < v.Len(); i++ {
			val, err := t.Elem.pack(v.Index(i))
			if err != nil {
				return nil, err
			}
			if !offsetReq {
				ret = append(ret, val...)
				continue
			}
			ret = append(ret, packNum(reflect.ValueOf(offset))...)
			offset += len(val)
			tail = append(tail, val...)
		}
		return append(ret, tail...), nil
	case TupleTy:
		
		
		
		
		
		
		
		
		
		fieldmap, err := mapArgNamesToStructFields(t.TupleRawNames, v)
		if err != nil {
			return nil, err
		}
		
		offset := 0
		for _, elem := range t.TupleElems {
			offset += getTypeSize(*elem)
		}
		var ret, tail []byte
		for i, elem := range t.TupleElems {
			field := v.FieldByName(fieldmap[t.TupleRawNames[i]])
			if !field.IsValid() {
				return nil, fmt.Errorf("field %s for tuple not found in the given struct", t.TupleRawNames[i])
			}
			val, err := elem.pack(field)
			if err != nil {
				return nil, err
			}
			if isDynamicType(*elem) {
				ret = append(ret, packNum(reflect.ValueOf(offset))...)
				tail = append(tail, val...)
				offset += len(val)
			} else {
				ret = append(ret, val...)
			}
		}
		return append(ret, tail...), nil

	default:
		return packElement(t, v)
	}
}



func (t Type) requiresLengthPrefix() bool {
	return t.T == StringTy || t.T == BytesTy || t.T == SliceTy
}








func isDynamicType(t Type) bool {
	if t.T == TupleTy {
		for _, elem := range t.TupleElems {
			if isDynamicType(*elem) {
				return true
			}
		}
		return false
	}
	return t.T == StringTy || t.T == BytesTy || t.T == SliceTy || (t.T == ArrayTy && isDynamicType(*t.Elem))
}









func getTypeSize(t Type) int {
	if t.T == ArrayTy && !isDynamicType(*t.Elem) {
		
		if t.Elem.T == ArrayTy || t.Elem.T == TupleTy {
			return t.Size * getTypeSize(*t.Elem)
		}
		return t.Size * 32
	} else if t.T == TupleTy && !isDynamicType(t) {
		total := 0
		for _, elem := range t.TupleElems {
			total += getTypeSize(*elem)
		}
		return total
	}
	return 32
}
