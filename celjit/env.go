package celjit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"

	"github.com/nicholasngai/cel-jit/runtime"
)

// EnvConfig configures the construction of an [Env].
type EnvConfig struct {
	// The types available to the runtime.
	Types []reflect.Type

	// The functions in the environment, keyed by their ID. Function IDs should
	// be lower_snake_case and unique.
	Functions map[string]Function
}

// Function is a definition of a function in CEL. A function in CEL may have
// several overloads (e.g. different parameters).
type Function struct {
	// The overloads of the function, keyed by their name. This name is used to
	// define the function or method name (e.g. startsWith).
	Overloads map[string]FunctionOverload
}

// FunctionOverload is a single overload of a [Function]. An overload contains a
// set of type parameters, a return type, and an implementation.
type FunctionOverload struct {
	// True if this is a member overload/method. False if this is a global
	// function.
	IsMemberOverload bool

	// The parameter types to the function. If this is a member overload, the
	// first parameter is the receiver.
	ParameterTypes []*cel.Type

	// The return type of the function.
	ReturnType *cel.Type

	// Implementation must be one of the two following types:
	//
	// - func(a T, b U[, ...]) R
	// - func(a T, b U[, ...]) (R, error)
	//
	// These type mappings are the same as [Compile].
	Implementation any
}

// Env represents a runtime environment for CEL. Like
// [github.com/cel-expr/cel-go/cel.Env], it contains definitions for functions
// that exist at runtime as well as type definitions. Unlike
// [github.com/cel-expr/cel-go/cel.Env], this only supports native types whose
// shape can be read via reflection.
type Env struct {
	config EnvConfig

	stdRuntimeDir    string
	customRuntimeDir string
	functions        map[string]envFunction

	// Plugin loading in Go takes up memory for the lifetime of the process so
	// we can't unload these, but we still want to dedupe programs with the same
	// source compiled against the same env. This allows duplicate programs to
	// still compile correctly---otherwise, if -trimpath is enabled, the
	// plugin/unnamed-%x will be the same per
	// https://cs.opensource.google/go/go/+/refs/tags/go1.26.5:src/cmd/go/internal/work/gc.go;l=559;bpv=1;bpt=0
	// and plugin loading will fail with "plugin already loaded" error.
	pluginsByHash sync.Map // [sha256.Size]byte -> func() (*plugin.Plugin, error)
}

type envFunction struct {
	dynRuntimeName string // The name used for the dynamic variant at custom.
	maxArguments   int    // The maximum # of arguments to the function in its overloads.
	isCustom       bool
	overloads      map[string]envFunctionOverload
}

type envFunctionOverload struct {
	config       FunctionOverload
	runtimeName  string
	returnsError bool
	isCustom     bool
}

func NewEnv(config EnvConfig) (_ *Env, retErr error) {
	// Make functions.
	functions := makeStandardEnv()
	for funcID, funcConfig := range config.Functions {
		function := functions[funcID]

		if function.overloads == nil {
			function.overloads = make(map[string]envFunctionOverload)
		}
		function.dynRuntimeName = fmt.Sprintf("custom.Func_%s", funcID)
		function.isCustom = true

		for overloadID, overloadConfig := range funcConfig.Overloads {
			function.overloads[overloadID] = envFunctionOverload{
				config:       overloadConfig,
				runtimeName:  fmt.Sprintf("custom.FuncOverload_%s", overloadID),
				returnsError: reflect.ValueOf(overloadConfig.Implementation).Type().NumOut() > 1,
				isCustom:     true,
			}
			function.maxArguments = max(function.maxArguments, len(overloadConfig.ParameterTypes))
		}

		functions[funcID] = function
	}

	stdRuntimeDir, err := getStdRuntimeDir()
	if err != nil {
		return nil, fmt.Errorf("write standard runtime: %w", err)
	}
	defer func() {
		if retErr != nil {
			closeStdRuntime()
		}
	}()

	customRuntimeDir, err := writeCustomRuntime(stdRuntimeDir, functions)
	if err != nil {
		return nil, fmt.Errorf("write custom runtime: %w", err)
	}

	return &Env{
		config: config,

		stdRuntimeDir:    stdRuntimeDir,
		customRuntimeDir: customRuntimeDir,
		functions:        functions,
	}, nil
}

func writeCustomRuntime(stdRuntimeDir string, functions map[string]envFunction) (_ string, retErr error) {
	// Skip if there are no custom functions.
	hasCustomFunction := false
outer:
	for _, function := range functions {
		if function.isCustom {
			hasCustomFunction = true
			break
		}
		for _, overload := range function.overloads {
			if overload.isCustom {
				hasCustomFunction = true
				break outer
			}
		}
	}
	if !hasCustomFunction {
		return "", nil
	}

	// Make runtime dir.
	dir, err := os.MkdirTemp("", "cel-jit-custom-runtime-source-*")
	if err != nil {
		return "", fmt.Errorf("mkdir temp: %w", err)
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(dir)
		}
	}()

	// Write Go module file.
	if err := writeFilef(filepath.Join(dir, "go.mod"),
		`module github.com/nicholasngai/cel-jit/%s

go 1.25.0

require github.com/nicholasngai/cel-jit/runtime v0.0.0-00010101000000-000000000000

replace github.com/nicholasngai/cel-jit/runtime => %s
`,
		filepath.Base(dir),
		stdRuntimeDir,
	); err != nil {
		return "", err
	}

	if err := os.Mkdir(filepath.Join(dir, "custom"), 0o755); err != nil {
		return "", fmt.Errorf("mkdir custom: %w", err)
	}

	// Write custom functions.
	if err := writeCustomFunctions(dir, functions); err != nil {
		return "", fmt.Errorf("custom functions: %w", err)
	}

	return dir, nil
}

func writeCustomFunctions(dir string, functions map[string]envFunction) (retErr error) {
	if len(functions) == 0 {
		return nil
	}

	// Create file.
	file, err := os.Create(filepath.Join(dir, "custom", "functions.go"))
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	// Write header.
	if _, err := freindentfLevel(file, 0, `
		package custom

		import (
			"fmt"
			"reflect"

			"github.com/nicholasngai/cel-jit/runtime"
		)

		func Dummy() {}
		`,
	); err != nil {
		return err
	}

	for funcID, function := range functions {
		if !function.isCustom {
			continue
		}

		for overloadID, overload := range function.overloads {
			if !overload.isCustom {
				continue
			}

			// Get runtime type info.
			paramTypeInfos := make([]runtimeTypeInfo, 0, len(overload.config.ParameterTypes))
			for i, paramType := range overload.config.ParameterTypes {
				paramTypeInfo, err := celTypeInfo(paramType)
				if err != nil {
					return fmt.Errorf("function %q: overload %q: parameter %d type: %w", funcID, overloadID, i, err)
				}
				paramTypeInfos = append(paramTypeInfos, paramTypeInfo)
			}
			returnTypeInfo, err := celTypeInfo(overload.config.ReturnType)
			if err != nil {
				return fmt.Errorf("function %q: overload %q: return type: %w", funcID, overloadID, err)
			}

			// Check if there is an error in the return type.
			var returnExpr string
			if overload.returnsError {
				returnExpr = fmt.Sprintf("(%s, error)", returnTypeInfo.goType)
			} else {
				returnExpr = returnTypeInfo.goType
			}

			// TODO(nngai) Sanitize names.
			if _, err := freindentfLevel(file, 0, `

				var %s func(%s) %s = runtime.LoadCustomFunction(%q, %[1]q).(func(%s) %s)
				`,
				strings.TrimPrefix(overload.runtimeName, "custom."),
				repeat("%s", paramTypeInfos, func(i int, t runtimeTypeInfo) []any { return []any{t.goType} }),
				returnExpr,
				filepath.Base(dir),
			); err != nil {
				return err
			}

			// Store custom function.
			runtime.StoreCustomFunction(filepath.Base(dir), strings.TrimPrefix(overload.runtimeName, "custom."), overload.config.Implementation)
		}

		// TODO(nngai) Sanitize names.
		if _, err := freindentfLevel(file, 0, `

			func %s(%s) (any, error) {
				switch [...]reflect.Type{%s} {
			`,
			strings.TrimPrefix(function.dynRuntimeName, "custom."),
			repeatInt("arg%d any", function.maxArguments, func(i int) []any { return []any{i} }),
			repeatInt("reflect.TypeOf(arg%d)", function.maxArguments, func(i int) []any { return []any{i} }),
		); err != nil {
			return err
		}
		for overloadID, overload := range function.overloads {
			paramTypeInfos := make([]runtimeTypeInfo, 0, len(overload.config.ParameterTypes))
			for i, paramType := range overload.config.ParameterTypes {
				paramTypeInfo, err := celTypeInfo(paramType)
				if err != nil {
					return fmt.Errorf("function %q: overload %q: param %d type info: %w", funcID, overloadID, i, err)
				}
				paramTypeInfos = append(paramTypeInfos, paramTypeInfo)
			}

			if _, err := freindentfLevel(file, 1, `
				case [...]reflect.Type{%s}:
				`,
				repeat("reflect.TypeFor[%s]()", paramTypeInfos, func(i int, paramTypeInfo runtimeTypeInfo) []any { return []any{paramTypeInfo.goType} }),
			); err != nil {
				return err
			}

			if overload.returnsError {
				if _, err := freindentfLevel(file, 2, `
					ret, err := %s(%s)
					return ret, err
					`,
					strings.TrimPrefix(overload.runtimeName, "custom."),
					repeat("arg%d.(%s)", paramTypeInfos, func(i int, paramTypeInfo runtimeTypeInfo) []any { return []any{i, paramTypeInfo.goType} }),
				); err != nil {
					return err
				}
			} else {
				if _, err := freindentfLevel(file, 2, `
					ret := %s(%s)
					return ret, nil
					`,
					strings.TrimPrefix(overload.runtimeName, "custom."),
					repeat("arg%d.(%s)", paramTypeInfos, func(i int, paramTypeInfo runtimeTypeInfo) []any { return []any{i, paramTypeInfo.goType} }),
				); err != nil {
					return err
				}
			}
		}
		if _, err := freindentfLevel(file, 0, `
				default:
					return nil, fmt.Errorf("unsupported type(s) %s", %s)
				}
			}
			`,
			repeatInt("%%T", function.maxArguments, func(i int) []any { return nil }),
			repeatInt("arg%d", function.maxArguments, func(i int) []any { return []any{i} }),
		); err != nil {
			return err
		}
	}

	return nil
}

// writeType performs a DFS on the given type and writes any type definitions
// traversed to the given file. It uses a map in order to memorize which types
// have already been traversed.
func writeType(f io.Writer, t reflect.Type, visited map[reflect.Type]string, nextStructIdx *int) error {
	if _, ok := visited[t]; ok {
		return nil
	}

	switch t.Kind() {
	case reflect.Int:
		visited[t] = "int"
	case reflect.Int8:
		visited[t] = "int8"
	case reflect.Int16:
		visited[t] = "int16"
	case reflect.Int32:
		visited[t] = "int32"
	case reflect.Int64:
		visited[t] = "int64"
	case reflect.Uint:
		visited[t] = "uint"
	case reflect.Uint8:
		visited[t] = "uint8"
	case reflect.Uint16:
		visited[t] = "uint16"
	case reflect.Uint32:
		visited[t] = "uint32"
	case reflect.Uint64:
		visited[t] = "uint64"
	case reflect.Uintptr:
		visited[t] = "uintptr"
	case reflect.Float32:
		visited[t] = "float32"
	case reflect.Float64:
		visited[t] = "float64"
	case reflect.Complex64:
		visited[t] = "complex64"
	case reflect.Complex128:
		visited[t] = "complex128"
	case reflect.Bool:
		visited[t] = "bool"
	case reflect.String:
		visited[t] = "string"
	case reflect.Interface:
		visited[t] = "any"
	case reflect.Pointer, reflect.Array, reflect.Slice, reflect.Chan:
		if err := writeType(f, t.Elem(), visited, nextStructIdx); err != nil {
			return err
		}
		switch t.Kind() {
		case reflect.Pointer:
			visited[t] = fmt.Sprintf("*%s", visited[t.Elem()])
		case reflect.Array:
			visited[t] = fmt.Sprintf("[%d]%s", t.Len(), visited[t.Elem()])
		case reflect.Slice:
			visited[t] = fmt.Sprintf("[]%s", visited[t.Elem()])
		case reflect.Chan:
			visited[t] = fmt.Sprintf("chan %s", visited[t.Elem()])
		}
	case reflect.Map:
		if err := writeType(f, t.Key(), visited, nextStructIdx); err != nil {
			return err
		}
		if err := writeType(f, t.Elem(), visited, nextStructIdx); err != nil {
			return err
		}
		visited[t] = fmt.Sprintf("map[%s]%s", visited[t.Key()], visited[t.Elem()])
	case reflect.Struct:
		structName := fmt.Sprintf("Struct_%s_%d", t.Name(), *nextStructIdx)
		visited[t] = structName
		*nextStructIdx += 1

		// Write all dependent types out first.
		for i := range t.NumField() {
			field := t.Field(i)
			if err := writeType(f, field.Type, visited, nextStructIdx); err != nil {
				return err
			}
		}

		// Write struct header.
		if _, err := fmt.Fprintf(f, "\ntype %s struct {\n", structName); err != nil {
			return err
		}

		// Write fields.
		for i := range t.NumField() {
			field := t.Field(i)
			if field.Anonymous {
				if _, err := fmt.Fprintf(f, "\t%s\n", visited[field.Type]); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(f, "\t%s %s\n", field.Name, visited[field.Type]); err != nil {
					return err
				}
			}
		}

		// Write struct footer.
		if _, err := fmt.Fprint(f, "}\n"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported type %v", t)
	}

	return nil
}

// Cleanup removes all filesystem artifacts generated by the environment.
func (e *Env) Cleanup() error {
	if e.customRuntimeDir != "" {
		if err := os.RemoveAll(e.customRuntimeDir); err != nil {
			return err
		}
	}
	closeStdRuntime()
	return nil
}
