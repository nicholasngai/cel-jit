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
	"sync"

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

	// Write runtime package.
	runtimeDir, err := writeRuntime()
	if err != nil {
		return nil, fmt.Errorf("write runtime: %w", err)
	}

	// Write Go module file.
	if err := writeFile(filepath.Join(tempDir, "go.mod"), fmt.Sprintf(
`module github.com/nicholasngai/cel-jit/compiled

go 1.26.0

require github.com/nicholasngai/cel-jit/runtime-source/runtime v0.0.0-00000000000000-000000000000

replace github.com/nicholasngai/cel-jit/runtime-source/runtime => %s
`, runtimeDir)); err != nil {
		return nil, err
	}

	// Write the program.
	if err := writeFile(filepath.Join(tempDir, "program.go"), fmt.Sprintf(
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

func Program(%s) (any, error) {
	val := program(%s)
	return val.Val(), val.Err()
}

func program(%s) runtime.Value {
	return %s
}
`,
		repeatParams("%s any", config.Parameters, mangleParameter),
		repeatParams("runtime.ValueOf(%s)", config.Parameters, mangleParameter),
		repeatParams("%s runtime.Value", config.Parameters, mangleVariable),
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

var runtimeDir string

var writeRuntime = sync.OnceValues(func() (string, error) {
	// Make runtime dir.
	dir, err := os.MkdirTemp("", "cel-jit-runtime-source-*")
	if err != nil {
		return "", fmt.Errorf("mkdir temp: %w", err)
	}

	// Write Go module file.
	if err := writeFile(filepath.Join(dir, "go.mod"),
`module github.com/nicholasngai/cel-jit/runtime-source/runtime

go 1.26.0
`); err != nil {
		return "", err
	}

	// Write the runtime to it.
	if err := os.MkdirAll(filepath.Join(dir, "runtime"), 0o755); err != nil {
		return "", fmt.Errorf("mkdir runtime: %w", err)
	}
	if err := writeFile(filepath.Join(dir, "runtime.go"), runtime.Source); err != nil {
		return "", err
	}

	runtimeDir = dir

	return dir, nil
})

func repeatParams(format string, params []Parameter, mangler func(string) string) string {
	var builder strings.Builder
	for i, param := range params {
		if i > 0 {
			builder.WriteString(", ")
		}
		if mangler != nil {
			builder.WriteString(fmt.Sprintf(format, mangler(param.Name)))
		} else {
			builder.WriteString(fmt.Sprintf(format, param.Name))
		}
	}
	return builder.String()
}

func astToGoSource(node *expr.Expr) (string, error) {
	switch exprKind := node.GetExprKind().(type) {
	case *expr.Expr_IdentExpr:
		return mangleVariable(exprKind.IdentExpr.GetName()), nil
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
	case *expr.Expr_SelectExpr:
		operandGo, err := astToGoSource(exprKind.SelectExpr.GetOperand())
		if err != nil {
			return "", fmt.Errorf("operand: %w", err)
		}
		if exprKind.SelectExpr.TestOnly {
			return fmt.Sprintf("runtime.Has(%s, %q)", operandGo, exprKind.SelectExpr.GetField()), nil
		}
		return fmt.Sprintf("runtime.Select(%s, %q)", operandGo, exprKind.SelectExpr.GetField()), nil
	case *expr.Expr_ComprehensionExpr:
		rangeGo, err := astToGoSource(exprKind.ComprehensionExpr.GetIterRange())
		if err != nil {
			return "", fmt.Errorf("range: %w", err)
		}

		accumulatorInitGo, err := astToGoSource(exprKind.ComprehensionExpr.GetAccuInit())
		if err != nil {
			return "", fmt.Errorf("accumulator init: %w", err)
		}

		loopStepGo, err := astToGoSource(exprKind.ComprehensionExpr.GetLoopStep())
		if err != nil {
			return "", fmt.Errorf("loop step: %w", err)
		}

		loopCondGo, err := astToGoSource(exprKind.ComprehensionExpr.GetLoopCondition())
		if err != nil {
			return "", fmt.Errorf("loop condition: %w", err)
		}

		resultGo, err := astToGoSource(exprKind.ComprehensionExpr.GetResult())
		if err != nil {
			return "", fmt.Errorf("result: %w", err)
		}

		return fmt.Sprintf(`(func() runtime.Value {
			collection := %[1]s
			if collection.Err() != nil {
				return collection
			}

			collectionVal := reflect.ValueOf(collection.Val())
			switch collectionVal.Type().Kind() {
			case reflect.Slice:
				%[2]s := %[3]s
				if %[2]s.Err() != nil {
					return %[2]s
				}

				for i := range collectionVal.Len() {
					%[4]s := runtime.ValueOf(collectionVal.Index(i).Interface())

					cond := %[5]s
					if cond.Err() != nil {
						return cond
					}
					if cond.Val() != true {
						break
					}

					%[2]s = %[6]s
					if %[2]s.Err() != nil {
						return %[2]s
					}
				}

				return %[7]s
			case reflect.Map:
				%[2]s := %[3]s
				if %[2]s.Err() != nil {
					return %[2]s
				}

				mapIter := collectionVal.MapRange()
				for mapIter.Next() {
					%[4]s := runtime.ValueOf(mapIter.Key().Interface())

					cond := %[5]s
					if cond.Err() != nil {
						return cond
					}
					if cond.Val() != true {
						break
					}

					%[2]s = %[6]s
					if %[2]s.Err() != nil {
						return %[2]s
					}
				}

				return %[7]s
			default:
				return runtime.ErrorOf(fmt.Errorf("unsupported comprehension type %%T", collectionVal))
			}
		})()`, rangeGo, mangleVariable(exprKind.ComprehensionExpr.GetAccuVar()), accumulatorInitGo, mangleVariable(exprKind.ComprehensionExpr.GetIterVar()), loopCondGo, loopStepGo, resultGo), nil
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
		case operators.Conditional:
			return fmt.Sprintf(`(func() runtime.Value {
				cond := %s
				if cond.Err() != nil {
					return cond
				}

				if cond.Val() == true {
					return %s
				} else {
					return %s
				}
			})()`, argsGo[0], argsGo[1], argsGo[2]), nil
		case operators.LogicalAnd:
			return fmt.Sprintf("runtime.LogicalAnd(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.LogicalOr:
			return fmt.Sprintf("runtime.LogicalOr(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.LogicalNot:
			return fmt.Sprintf("runtime.LogicalNot(%s)", argsGo[0]), nil
		case operators.Equals:
			return fmt.Sprintf("runtime.Equals(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.NotEquals:
			return fmt.Sprintf("runtime.NotEquals(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Less:
			return fmt.Sprintf("runtime.Less(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.LessEquals:
			return fmt.Sprintf("runtime.LessEquals(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Greater:
			return fmt.Sprintf("runtime.Greater(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.GreaterEquals:
			return fmt.Sprintf("runtime.GreaterEquals(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Add:
			return fmt.Sprintf("runtime.Add(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Subtract:
			return fmt.Sprintf("runtime.Subtract(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Multiply:
			return fmt.Sprintf("runtime.Multiply(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Divide:
			return fmt.Sprintf("runtime.Divide(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Modulo:
			return fmt.Sprintf("runtime.Modulo(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.Negate:
			return fmt.Sprintf("runtime.Negate(%s)", argsGo[0]), nil
		case operators.Index:
			return fmt.Sprintf("runtime.Index(%s, %s)", argsGo[0], argsGo[1]), nil
		case operators.NotStrictlyFalse:
			return fmt.Sprintf("runtime.NotStrictlyFalse(%s)", argsGo[0]), nil
		case operators.In:
			return fmt.Sprintf("runtime.In(%s, %s)", argsGo[0], argsGo[1]), nil
		case "dyn":
			return argsGo[0], nil
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

func mangleParameter(varName string) string {
	// Replace periods.
	varName = strings.ReplaceAll(varName, "_", "__")
	varName = strings.ReplaceAll(varName, ".", "_dot_")
	return varName
}

func mangleVariable(varName string) string {
	varName = mangleParameter(varName)

	// These must return distinct prefixes.
	if trimmed, ok := strings.CutPrefix(varName, "@"); ok {
		return fmt.Sprintf("var_at__%s", trimmed)
	} else {
		return fmt.Sprintf("var__%s", varName)
	}
}

// Cleanup removes all filesystem artifacts generated by cel-jit.
func Cleanup() error {
	if runtimeDir != "" {
		if err := os.RemoveAll(runtimeDir); err != nil {
			return err
		}
	}
	return nil
}
