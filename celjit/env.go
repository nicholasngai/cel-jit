package celjit

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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

	runtimeDir string
	functions  map[string]envFunction

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
	dynRuntimeName string // The name used for the dynamic variant at runtime.
	maxArguments   int    // The maximum # of arguments to the function in its overloads.
	overloads      map[string]envFunctionOverload
}

type envFunctionOverload struct {
	runtimeName      string
	isMemberOverload bool
	parameterTypes   []*cel.Type
	returnType       *cel.Type
}

func NewEnv(config EnvConfig) (*Env, error) {
	runtimeDir, err := writeRuntime()
	if err != nil {
		return nil, fmt.Errorf("write runtime: %w", err)
	}

	return &Env{
		config: config,

		runtimeDir: runtimeDir,
		functions:  makeStandardEnv(), // TODO(nngai) Use custom overloads.
	}, nil
}

func writeRuntime() (_ string, retErr error) {
	// Make runtime dir.
	dir, err := os.MkdirTemp("", "cel-jit-runtime-source-*")
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

go 1.26.0
`,
		filepath.Base(dir),
	); err != nil {
		return "", err
	}

	// Write the runtime to it.
	if err := fs.WalkDir(runtime.Source, ".", func(runtimePath string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry.IsDir() {
			if err := os.Mkdir(filepath.Join(dir, "runtime", runtimePath), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Join(dir, "runtime", runtimePath), err)
			}
		} else {
			contents, err := runtime.Source.ReadFile(runtimePath)
			if err != nil {
				return fmt.Errorf("read embedded %s: %w", runtimePath, err)
			}
			if err := writeFilef(filepath.Join(dir, "runtime", runtimePath), "%s", string(contents)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return "", err
	}

	return dir, nil
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
		for field := range t.Fields() {
			if err := writeType(f, field.Type, visited, nextStructIdx); err != nil {
				return err
			}
		}

		// Write struct header.
		if _, err := fmt.Fprintf(f, "\ntype %s struct {\n", structName); err != nil {
			return err
		}

		// Write fields.
		for field := range t.Fields() {
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
	if err := os.RemoveAll(e.runtimeDir); err != nil {
		return err
	}
	return nil
}
