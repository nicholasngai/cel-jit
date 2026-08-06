package celjit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"runtime/debug"
	"strings"

	"github.com/google/cel-go/cel"
)

// Config is the config for a [Compile] call.
type Config struct {
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

// celTypeToRuntimeTypes maps CEL types to their JIT runtime types.
func celTypeToRuntimeTypes(t *cel.Type) (goType string, runtimeType string, _ error) {
	switch t {
	case cel.DynType:
		return "any", "DynValue", nil
	case cel.IntType:
		return "int64", "IntValue", nil
	default:
		return "", "", fmt.Errorf("unhandled type %v", t)
	}
}

// Compile returns a JIT-compiled version of the given CEL expression. For each
// parameter, [Config.Parameters], the returned function will be of type
//
//	func(a any, b any[, ...]) (any, error)
func Compile(config Config) ([]any, error) {
	// Make a temporary compile directory.
	tempDir, err := os.MkdirTemp("", "cel-jit-compiled-*")
	if err != nil {
		return nil, fmt.Errorf("mkdir temp: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write runtime package.
	runtimeDir, err := writeRuntime()
	if err != nil {
		return nil, fmt.Errorf("write runtime: %w", err)
	}

	// Write Go module file.
	if err := writeFilef(filepath.Join(tempDir, "go.mod"),
`module github.com/nicholasngai/cel-jit/compiled

go 1.26.0

require github.com/nicholasngai/cel-jit/runtime-source v0.0.0-00000000000000-000000000000

replace github.com/nicholasngai/cel-jit/runtime-source => %s
`,
	runtimeDir); err != nil {
		return nil, err
	}

	// Write program header.
	programFile, err := os.Create(filepath.Join(tempDir, "program.go"))
	if err != nil {
		return nil, fmt.Errorf("create program.go: %w", err)
	}
	defer programFile.Close()
	if _, err := programFile.WriteString(
`package main

import (
	"fmt"
	"reflect"

	"github.com/nicholasngai/cel-jit/runtime-source/runtime"
)

var (
	_ = fmt.Print
	_ = reflect.ValueOf
)
`,
	); err != nil {
		return nil, fmt.Errorf("write program.go: %w", err)
	}

	// Append each expression to program file.
	for i, exprConfig := range config.Exprs {
		envOptions := make([]cel.EnvOption, 0, len(exprConfig.Parameters)+2)
		envOptions = append(envOptions,
			cel.EagerlyValidateDeclarations(true),
			cel.ExtendedValidations(),
		)

		// Make variables.
		for _, param := range exprConfig.Parameters {
			envOptions = append(envOptions, cel.Variable(param.Name, param.Type))
		}

		// Make CEL env.
		env, err := cel.NewEnv(envOptions...)
		if err != nil {
			return nil, fmt.Errorf("CEL env: %w", err)
		}

		// Compile and check return type.
		ast, iss := env.Compile(exprConfig.Expr)
		if err := iss.Err(); err != nil {
			return nil, fmt.Errorf("CEL compile: %w", err)
		}
		if exprConfig.ReturnType != cel.DynType && !ast.OutputType().IsAssignableType(exprConfig.ReturnType) {
			return nil, fmt.Errorf("CEL return type %v does not match expected return type %v", ast.OutputType(), exprConfig.ReturnType)
		}

		// Get AST.
		astExpr, err := cel.AstToCheckedExpr(ast)
		if err := iss.Err(); err != nil {
			return nil, fmt.Errorf("CEL checked expr to AST: %w", err)
		}

		// Make Go source.
		goSource, err := astToGoSource(astExpr.GetExpr())
		if err != nil {
			return nil, fmt.Errorf("generate Go source: %w", err)
		}

		// Get runtime types.
		type runtimeParameter struct {
			parameter   Parameter
			goType      string
			runtimeType string
		}
		runtimeParameters := make([]runtimeParameter, 0, len(exprConfig.Parameters))
		for _, parameter := range exprConfig.Parameters {
			goType, runtimeType, err := celTypeToRuntimeTypes(parameter.Type)
			if err != nil {
				return nil, fmt.Errorf("parameter %q type: %w", parameter.Name, err)
			}
			runtimeParameters = append(runtimeParameters, runtimeParameter{
				parameter: parameter,
				goType: goType,
				runtimeType: runtimeType,
			})
		}
		returnGoType, returnRuntimeType, err := celTypeToRuntimeTypes(exprConfig.ReturnType)
		if err != nil {
			return nil, fmt.Errorf("return type: %w", err)
		}

		// Write the program.
		if _, err := fmt.Fprintf(programFile,
`
func Program%d(%s) (%s, error) {
	val := program%[1]d(%[4]s)
	return val.Val(), val.Err()
}

func program%[1]d(%[5]s) runtime.%s {
	return %s.%s()
}
`,
			i,
			repeat("%s %s", runtimeParameters, func(r runtimeParameter) []any {
				return []any{mangleParameter(r.parameter.Name), r.goType}
			}),
			returnGoType,
			repeat("runtime.%sOf(%s)", runtimeParameters, func(r runtimeParameter) []any {
				return []any{r.runtimeType, mangleParameter(r.parameter.Name)}
			}),
			repeat("%s runtime.%s", runtimeParameters, func(r runtimeParameter) []any {
				return []any{mangleVariable(r.parameter.Name), r.runtimeType}
			}),
			returnRuntimeType,
			goSource,
			returnRuntimeType,
		); err != nil {
			return nil, err
		}
	}

	if err := programFile.Close(); err != nil {
		return nil, fmt.Errorf("close program.go: %w", err)
	}

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

func writeFilef(path string, format string, args... any) error {
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
