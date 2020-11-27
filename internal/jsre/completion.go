















package jsre

import (
	"sort"
	"strings"

	"github.com/dop251/goja"
)



func (jsre *JSRE) CompleteKeywords(line string) []string {
	var results []string
	jsre.Do(func(vm *goja.Runtime) {
		results = getCompletions(vm, line)
	})
	return results
}

func getCompletions(vm *goja.Runtime, line string) (results []string) {
	parts := strings.Split(line, ".")
	if len(parts) == 0 {
		return nil
	}

	
	
	obj := vm.GlobalObject()
	for i := 0; i < len(parts)-1; i++ {
		v := obj.Get(parts[i])
		if v == nil {
			return nil 
		}
		obj = v.ToObject(vm)
	}

	
	
	
	prefix := parts[len(parts)-1]
	iterOwnAndConstructorKeys(vm, obj, func(k string) {
		if strings.HasPrefix(k, prefix) {
			if len(parts) == 1 {
				results = append(results, k)
			} else {
				results = append(results, strings.Join(parts[:len(parts)-1], ".")+"."+k)
			}
		}
	})

	
	
	if len(results) == 1 && results[0] == line {
		obj := obj.Get(parts[len(parts)-1])
		if obj != nil {
			if _, isfunc := goja.AssertFunction(obj); isfunc {
				results[0] += "("
			} else {
				results[0] += "."
			}
		}
	}

	sort.Strings(results)
	return results
}
