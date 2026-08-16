package runtime

import "sync"

var customFunctions sync.Map // map[string]chan map[string]any

type customFunctionKey struct {
	customRuntimeName string
	functionName      string
}

func StoreCustomFunction(customRuntimeName string, functionName string, function any) {
	customFunctions.Store(customFunctionKey{customRuntimeName, functionName}, function)
}

func LoadCustomFunction(customRuntimeName string, functionName string) any {
	v, _ := customFunctions.LoadAndDelete(customFunctionKey{customRuntimeName, functionName})
	return v
}
