package celjit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/operators"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"

	"github.com/nicholasngai/cel-jit/runtime"
)

type Config struct {
	Parameters []Parameter
}

type Parameter struct {
	Name string
	Type *cel.Type
}

// Compile returns a JIT-compiled version of the given CEL expression. For each
// parameter, [Config.Parameters], the returned function will be of type
//
//	func(a any, b any[, ...]) (any, error)
func Compile(expr string, config Config) (any, error) {
	envOptions := make([]cel.EnvOption, 0, len(config.Parameters)+2)
	envOptions = append(envOptions,
		cel.EagerlyValidateDeclarations(true),
		cel.ExtendedValidations(),
	)

	// Make variables.
	for _, param := range config.Parameters {
		envOptions = append(envOptions, cel.Variable(param.Name, param.Type))
	}

	// Make CEL env.
	env, err := cel.NewEnv(envOptions...)
	if err != nil {
		return nil, fmt.Errorf("CEL env: %w", err)
	}

	// Parse AST.
	ast, iss := env.Compile(expr)
	if err := iss.Err(); err != nil {
		return nil, fmt.Errorf("CEL compile: %w", err)
	}
	astExpr, err := cel.AstToCheckedExpr(ast)
	if err := iss.Err(); err != nil {
		return nil, fmt.Errorf("CEL checked expr to AST: %w", err)
	}

	// Make Go source.
	goSource, err := astToGoSource(astExpr.GetExpr())
	if err != nil {
		return nil, fmt.Errorf("generate Go source: %w", err)
	}

	// Make a temporary compile directory.
	tempDir, err := os.MkdirTemp("", "cel-jit-compiled-*")
	if err != nil {
		return nil, fmt.Errorf("mkdir temp: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write Go module file.
	if err := writeFile(filepath.Join(tempDir, "go.mod"), fmt.Sprintf(
`module github.com/nicholasngai/cel-jit/%s

go 1.26
`, filepath.Base(tempDir))); err != nil {
		return nil, err
	}

	// Write the runtime to it.
	if err := os.MkdirAll(filepath.Join(tempDir, "runtime"), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir runtime: %w", err)
	}
	if err := writeFile(filepath.Join(tempDir, "runtime", "runtime.go"), runtime.Source); err != nil {
		return nil, err
	}

	// Write the program.
	if err := writeFile(filepath.Join(tempDir, "program.go"), fmt.Sprintf(
`package main

import (
	"github.com/nicholasngai/cel-jit/%s/runtime"
)

func Program(%s) (any, error) {
	val := program(%s)
	return val.Val(), val.Err()
}

func program(%s) runtime.Value {
	return %s
}
`,
		filepath.Base(tempDir),
		repeatParams("%s any", config.Parameters),
		repeatParams("runtime.ValueOf(%s)", config.Parameters),
		repeatParams("%s runtime.Value", config.Parameters),
		goSource,
	)); err != nil {
		return nil, err
	}

	// Compile it.
	cmd := exec.Command("go", "build",
		"-C", tempDir,
		"-buildmode=plugin",
		"-o", "program.so",
		"program.go",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go build program.go: %w\n\n%s", err, string(output))
	}

	// Load it into memory.
	plug, err := plugin.Open(filepath.Join(tempDir, "program.so"))
	if err != nil {
		return nil, fmt.Errorf("load program.so: %w", err)
	}
	program, err := plug.Lookup("Program")
	if err != nil {
		return nil, fmt.Errorf("lookup program: %w", err)
	}

	return program, nil
}

func repeatParams(format string, params []Parameter) string {
	var builder strings.Builder
	for i, param := range params {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(fmt.Sprintf(format, param.Name))
	}
	return builder.String()
}

func astToGoSource(node *expr.Expr) (string, error) {
	switch exprKind := node.GetExprKind().(type) {
	case *expr.Expr_IdentExpr:
		return exprKind.IdentExpr.GetName(), nil
	case *expr.Expr_ConstExpr:
		switch constKind := exprKind.ConstExpr.GetConstantKind().(type) {
		case *expr.Constant_Int64Value:
			return fmt.Sprintf("runtime.ValueOf(int64(%d))", constKind.Int64Value), nil
		case *expr.Constant_Uint64Value:
			return fmt.Sprintf("runtime.ValueOf(uint64(%d))", constKind.Uint64Value), nil
		case *expr.Constant_DoubleValue:
			return fmt.Sprintf("runtime.ValueOf(%f)", constKind.DoubleValue), nil
		case *expr.Constant_BoolValue:
			return fmt.Sprintf("runtime.ValueOf(%t)", constKind.BoolValue), nil
		case *expr.Constant_StringValue:
			return fmt.Sprintf("runtime.ValueOf(%q)", constKind.StringValue), nil
		case *expr.Constant_BytesValue:
			var builder strings.Builder
			builder.WriteString("runtime.ValueOf([]byte{")
			for i, b := range constKind.BytesValue {
				if i > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(strconv.Itoa(int(b)))
			}
			builder.WriteString("})")
			return builder.String(), nil
		case *expr.Constant_NullValue:
			return "runtime.ValueOf(nil)", nil
		default:
			return "", fmt.Errorf("unsupported constant kind %q", exprKind.ConstExpr.GetConstantKind())
		}
	case *expr.Expr_ListExpr:
		var builder strings.Builder
		builder.WriteString("runtime.ValueOfSlice(func(yield func(v runtime.Value) bool) {")
		for i, elem := range exprKind.ListExpr.GetElements() {
			if i > 0 {
				builder.WriteString("; ")
			}

			elemSource, err := astToGoSource(elem)
			if err != nil {
				return "", fmt.Errorf("list elem %d: %w", i, err)
			}
			builder.WriteString("if !yield(")
			builder.WriteString(elemSource)
			builder.WriteString(") { return }")
		}
		builder.WriteString("}, ")
		builder.WriteString(strconv.Itoa(len(exprKind.ListExpr.GetElements())))
		builder.WriteString(")")
		return builder.String(), nil
	case *expr.Expr_StructExpr:
		if exprKind.StructExpr.MessageName == "" {
			// Map.
			var builder strings.Builder
			builder.WriteString("runtime.ValueOfMap(func(yield func(key, value runtime.Value) bool) {")
			for i, entry := range exprKind.StructExpr.GetEntries() {
				if i > 0 {
					builder.WriteString("; ")
				}

				keySource, err := astToGoSource(entry.GetMapKey())
				if err != nil {
					return "", fmt.Errorf("map key %d: %w", i, err)
				}
				valSource, err := astToGoSource(entry.GetValue())
				if err != nil {
					return "", fmt.Errorf("map value %d: %w", i, err)
				}
				builder.WriteString("if !yield(")
				builder.WriteString(keySource)
				builder.WriteString(", ")
				builder.WriteString(valSource)
				builder.WriteString(") { return }")
			}
			builder.WriteString("}, ")
			builder.WriteString(strconv.Itoa(len(exprKind.StructExpr.GetEntries())))
			builder.WriteString(")")
			return builder.String(), nil
			} else {
				// Message.
				return "", errors.New("message literals unsupported")
			}
	case *expr.Expr_CallExpr:
		// Arguments.
		argsGo := make([]string, 0, len(exprKind.CallExpr.GetArgs()))
		for i, arg := range exprKind.CallExpr.GetArgs() {
			argGo, err := astToGoSource(arg)
			if err != nil {
				return "", fmt.Errorf("args[%d]: %w", i, err)
			}
			argsGo = append(argsGo, argGo)
		}

		switch exprKind.CallExpr.GetFunction() {
		case operators.Add:
			return fmt.Sprintf("runtime.Add(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Equals:
			return fmt.Sprintf("runtime.Eq(%s, %s)", argsGo[0], argsGo[1]), nil
		default:
			return "", fmt.Errorf("unsupported function %q", exprKind.CallExpr.GetFunction())
		}
	default:
		return "", fmt.Errorf("unsupported expr kind %v", node)
	}
}

func writeFile(path string, contents string) error {
	runtimeFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer runtimeFile.Close()

	if _, err := runtimeFile.WriteString(contents); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	if err := runtimeFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}

	return nil
}
