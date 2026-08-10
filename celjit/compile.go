package celjit

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// CompileConfig is the config for a [Compile] call.
type CompileConfig struct {
	Exprs []ExprConfig
}

// ExprConfig is the config for a single expression to be compiled.
type ExprConfig struct {
	Expr       string
	Parameters []Parameter
	ReturnType *cel.Type
}

// Parameter is a CEL parameter with a name and a type.
type Parameter struct {
	Name string
	Type *cel.Type
}

type runtimeTypeInfo struct {
	goType      string
	runtimeType string
	converter   func(string) string
	equaler     func(string, string) string
}

// celTypeInfo maps CEL types to their JIT runtime types.
func celTypeInfo(t *cel.Type) (runtimeTypeInfo, error) {
	switch t.Kind() {
	case cel.ListKind:
		elemTypeInfo, err := celTypeInfo(t.Parameters()[0])
		if err != nil {
			return runtimeTypeInfo{}, fmt.Errorf("list[0]: %w", err)
		}
		return runtimeTypeInfo{
			goType:      fmt.Sprintf("[]%s", elemTypeInfo.goType),
			runtimeType: fmt.Sprintf("runtime.ListValue[%s]", elemTypeInfo.goType),
			converter: func(s string) string {
				return fmt.Sprintf("runtime.ToListValue[%s](%s)", elemTypeInfo.goType, s)
			},
			equaler: func(a, b string) string {
				return fmt.Sprintf("slices.EqualFunc(%s, %s, func(a, b %s) bool { return %s })", a, b, elemTypeInfo.goType, elemTypeInfo.equaler("a", "b"))
			},
		}, nil
	case cel.MapKind:
		keyTypeInfo, err := celTypeInfo(t.Parameters()[0])
		if err != nil {
			return runtimeTypeInfo{}, fmt.Errorf("map[0]: %w", err)
		}
		valTypeInfo, err := celTypeInfo(t.Parameters()[1])
		if err != nil {
			return runtimeTypeInfo{}, fmt.Errorf("map[1]: %w", err)
		}
		return runtimeTypeInfo{
			goType:      fmt.Sprintf("map[%s]%s", keyTypeInfo.goType, valTypeInfo.goType),
			runtimeType: fmt.Sprintf("runtime.MapValue[%s, %s]", keyTypeInfo.goType, valTypeInfo.goType),
			converter: func(s string) string {
				return fmt.Sprintf("runtime.ToMapValue[%s, %s](%s)", keyTypeInfo.goType, valTypeInfo.goType, s)
			},
			equaler: func(a, b string) string {
				return fmt.Sprintf("maps.EqualFunc(%s, %s, func(a, b %s) bool { return %s })", a, b, valTypeInfo.goType, valTypeInfo.equaler("a", "b"))
			},
		}, nil
	}

	switch t {
	case cel.DynType:
		return runtimeTypeInfo{
			goType:      "any",
			runtimeType: "runtime.DynValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.DynValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("runtime.Eq(%s, %s)", a, b) },
		}, nil
	case cel.IntType:
		return runtimeTypeInfo{
			goType:      "int64",
			runtimeType: "runtime.IntValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.IntValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("(%s == %s)", a, b) },
		}, nil
	case cel.UintType:
		return runtimeTypeInfo{
			goType:      "uint64",
			runtimeType: "runtime.UintValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.UintValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("(%s == %s)", a, b) },
		}, nil
	case cel.DoubleType:
		return runtimeTypeInfo{
			goType:      "float64",
			runtimeType: "runtime.DoubleValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.DoubleValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("(%s == %s)", a, b) },
		}, nil
	case cel.BoolType:
		return runtimeTypeInfo{
			goType:      "bool",
			runtimeType: "runtime.BoolValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.BoolValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("(%s == %s)", a, b) },
		}, nil
	case cel.StringType:
		return runtimeTypeInfo{
			goType:      "string",
			runtimeType: "runtime.StringValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.StringValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("(%s == %s)", a, b) },
		}, nil
	case cel.BytesType:
		return runtimeTypeInfo{
			goType:      "[]byte",
			runtimeType: "runtime.BytesValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.BytesValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("slices.Equal(%s, %s)", a, b) },
		}, nil
	case cel.TimestampType:
		return runtimeTypeInfo{
			goType:      "time.Time",
			runtimeType: "runtime.TimestampValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.TimestampValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("%s.Equal(%s)", a, b) },
		}, nil
	case cel.DurationType:
		return runtimeTypeInfo{
			goType:      "time.Duration",
			runtimeType: "runtime.DurationValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.DurationValue()", s) },
			equaler:     func(a, b string) string { return fmt.Sprintf("(%s == %s))", a, b) },
		}, nil
	case cel.NullType:
		return runtimeTypeInfo{
			goType:      "struct{}",
			runtimeType: "runtime.NullValue",
			converter:   func(s string) string { return fmt.Sprintf("%s.NullValue()", s) },
			equaler:     func(a, b string) string { return "true" },
		}, nil
	default:
		return runtimeTypeInfo{}, fmt.Errorf("unhandled type %v", t)
	}
}

// Plugin loading in Go takes up memory for the lifetime of the process anyway,
// so we might as well keep a hash of program file -> loaded plugin. This allows
// duplicate programs to still compile correctly---otherwise, if -trimpath is
// enabled, the plugin/unnamed-%x will be the same per
// https://cs.opensource.google/go/go/+/refs/tags/go1.26.5:src/cmd/go/internal/work/gc.go;l=559;bpv=1;bpt=0
// and plugin loading will fail with "plugin already loaded" error.
var pluginsByHash sync.Map // [sha256.Size]byte -> func() (*plugin.Plugin, error)

// Compile returns a JIT-compiled version of the given CEL expression. For each
// parameter, [CompileConfig.Parameters], the returned function will be of type
//
//	func(a T, b U[, ...]) (R, error)
//
// Parameter types and the return type are chosen based on the type paraemters.
// The mapping of CEL parameter to Go type is as follows:
//
// - int -> int64
// - uint -> uint64
// - double -> float64
// - bool -> bool
// - string -> string
// - bytes -> []byte
// - list(T) -> []T
// - map(K, V) -> map[K]V
// - timestamp -> [time.Time]
// - duration -> [time.Duration]
// - dyn -> any
func (e *Env) Compile(config CompileConfig) ([]any, error) {
	// Compile plugin.
	plug, err := e.compilePlugin(config)
	if err != nil {
		return nil, err
	}

	// Return functions.
	funcs := make([]any, 0, len(config.Exprs))
	for i := range config.Exprs {
		f, err := plug.Lookup(fmt.Sprintf("Program%d", i))
		if err != nil {
			return nil, fmt.Errorf("lookup program: %w", err)
		}
		funcs = append(funcs, f)
	}

	return funcs, nil
}

func (e *Env) compilePlugin(config CompileConfig) (*plugin.Plugin, error) {
	// Make a temporary compile directory.
	tempDir, err := os.MkdirTemp("", "cel-jit-compiled-*")
	if err != nil {
		return nil, fmt.Errorf("mkdir temp: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write Go module file.
	if err := writeFilef(filepath.Join(tempDir, "go.mod"),
		`module github.com/nicholasngai/cel-jit/compiled

go 1.26.0

require github.com/nicholasngai/cel-jit/runtime-source v0.0.0-00000000000000-000000000000

replace github.com/nicholasngai/cel-jit/runtime-source => %s
`,
		e.runtimeDir,
	); err != nil {
		return nil, err
	}

	// Write file out.
	programHasher := sha256.New()
	programFile, err := os.Create(filepath.Join(tempDir, "program.go"))
	if err != nil {
		return nil, fmt.Errorf("create program.go: %w", err)
	}
	defer programFile.Close()
	program := io.MultiWriter(programHasher, programFile)

	// Write header.
	if _, err := fmt.Fprint(program,
		`package main

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/nicholasngai/cel-jit/runtime-source/runtime"
)

var (
	_ = fmt.Print
	_ = maps.Equal[map[int]int, map[int]int]
	_ = reflect.ValueOf
	_ = slices.Equal[[]int]
	_ = time.Parse
)
`,
	); err != nil {
		return nil, fmt.Errorf("write program.go: %w", err)
	}

	// Append each expression to program file.
	for i, exprConfig := range config.Exprs {
		envOptions := make([]cel.EnvOption, 0, len(e.config.Functions)+len(exprConfig.Parameters)+3)
		envOptions = append(envOptions,
			cel.EagerlyValidateDeclarations(true),
			cel.ExtendedValidations(),
		)

		// Make types.
		typesAny := make([]any, 0, len(e.config.Types))
		for _, t := range e.config.Types {
			typesAny = append(typesAny, t)
		}
		envOptions = append(envOptions, ext.NativeTypes(typesAny...))

		// Make functions.
		for funcID, funcConfig := range e.config.Functions {
			funcOpts := make([]cel.FunctionOpt, 0, len(funcConfig.Overloads))
			for overloadID, overloadConfig := range funcConfig.Overloads {
				if overloadConfig.IsMemberOverload {
					funcOpts = append(funcOpts, cel.MemberOverload(overloadID, overloadConfig.ParameterTypes, overloadConfig.ReturnType))
				} else {
					funcOpts = append(funcOpts, cel.Overload(overloadID, overloadConfig.ParameterTypes, overloadConfig.ReturnType))
				}
			}
			envOptions = append(envOptions, cel.Function(funcID, funcOpts...))
		}

		// Make variables.
		for _, param := range exprConfig.Parameters {
			envOptions = append(envOptions, cel.Variable(param.Name, param.Type))
		}

		// Make CEL env.
		env, err := cel.NewEnv(envOptions...)
		if err != nil {
			return nil, fmt.Errorf("%q: CEL env: %w", exprConfig.Expr, err)
		}

		// Compile and check return type.
		ast, iss := env.Compile(exprConfig.Expr)
		if err := iss.Err(); err != nil {
			return nil, fmt.Errorf("%q: CEL compile: %w", exprConfig.Expr, err)
		}
		if exprConfig.ReturnType != cel.DynType && !ast.OutputType().IsAssignableType(exprConfig.ReturnType) {
			return nil, fmt.Errorf("%q: CEL return type %v does not match expected return type %v", exprConfig.Expr, ast.OutputType(), exprConfig.ReturnType)
		}

		// Get AST.
		astExpr, err := cel.AstToCheckedExpr(ast)
		if err := iss.Err(); err != nil {
			return nil, fmt.Errorf("%q: CEL checked expr to AST: %w", exprConfig.Expr, err)
		}

		// Make Go source.
		goSource, err := e.astToGoSource(astExpr.GetExpr(), astExpr)
		if err != nil {
			return nil, fmt.Errorf("%q: generate Go source: %w", exprConfig.Expr, err)
		}

		// Get runtime types.
		type runtimeParameter struct {
			parameter Parameter
			typeInfo  runtimeTypeInfo
		}
		runtimeParameters := make([]runtimeParameter, 0, len(exprConfig.Parameters))
		for _, parameter := range exprConfig.Parameters {
			paramTypeInfo, err := celTypeInfo(parameter.Type)
			if err != nil {
				return nil, fmt.Errorf("%q: parameter %q type: %w", exprConfig.Expr, parameter.Name, err)
			}
			runtimeParameters = append(runtimeParameters, runtimeParameter{
				parameter: parameter,
				typeInfo:  paramTypeInfo,
			})
		}
		returnTypeInfo, err := celTypeInfo(exprConfig.ReturnType)
		if err != nil {
			return nil, fmt.Errorf("%q: return type: %w", exprConfig.Expr, err)
		}

		// Write the program.
		if _, err := fmt.Fprintf(program,
			`
func Program%d(%s) (%s, error) {
	val := program%[1]d(%[4]s)
	return val.Val, val.Err
}

func program%[1]d(%[5]s) %s {
	return %s
}
`,
			i,
			repeat("%s %s", runtimeParameters, func(r runtimeParameter) []any {
				return []any{mangleParameter(r.parameter.Name), r.typeInfo.goType}
			}),
			returnTypeInfo.goType,
			repeat("%s{Val: %s}", runtimeParameters, func(r runtimeParameter) []any {
				return []any{r.typeInfo.runtimeType, mangleParameter(r.parameter.Name)}
			}),
			repeat("%s %s", runtimeParameters, func(r runtimeParameter) []any {
				return []any{mangleVariable(r.parameter.Name), r.typeInfo.runtimeType}
			}),
			returnTypeInfo.runtimeType,
			returnTypeInfo.converter(goSource),
		); err != nil {
			return nil, err
		}
	}

	if err := programFile.Close(); err != nil {
		return nil, fmt.Errorf("close program.go: %w", err)
	}

	programHashSlice := programHasher.Sum(nil)
	var programHash [sha256.Size]byte
	copy(programHash[:], programHashSlice)

	// Make sync.OnceValues and put it in the cache. We use LoadOrStore to
	// dedupe with anyone else that might be compiling this hash at the same
	// time.
	pluginFunc := sync.OnceValues(func() (*plugin.Plugin, error) {
		// Compile it. Use the same build args as the currently running binary, other than -buildmode.
		goArgs := []string{
			"build",
			"-C", tempDir,
			"-buildmode=plugin",
		}
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range buildInfo.Settings {
				if strings.HasPrefix(setting.Key, "-") && setting.Key != "-buildmode" {
					goArgs = append(goArgs, fmt.Sprintf("%s=%s", setting.Key, setting.Value))
				}
			}
		}
		goArgs = append(goArgs,
			"-o", "program.so",
			"program.go",
		)
		cmd := exec.Command("go", goArgs...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("go build program.go: %w\n\n%s", err, string(output))
		}

		// Load program into memory.
		plug, err := plugin.Open(filepath.Join(tempDir, "program.so"))
		if err != nil {
			return nil, fmt.Errorf("load program.so: %w", err)
		}

		return plug, nil
	})
	pluginFuncAny, _ := pluginsByHash.LoadOrStore(programHash, pluginFunc)
	pluginFunc = pluginFuncAny.(func() (*plugin.Plugin, error))

	return pluginFunc()
}

func repeat[T any](format string, vals []T, mapper func(T) []any) string {
	var builder strings.Builder
	for i, val := range vals {
		if i > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(&builder, format, mapper(val)...)
	}
	return builder.String()
}

func writeFilef(path string, format string, args ...any) error {
	runtimeFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer runtimeFile.Close()

	if _, err := fmt.Fprintf(runtimeFile, format, args...); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	if err := runtimeFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}

	return nil
}
