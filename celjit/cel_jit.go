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

func repeatParams(format string, params []Parameter, mangler func(string) string) string {
	var builder strings.Builder
	for i, param := range params {
		if i > 0 {
			builder.WriteString(", ")
		}
		if mangler != nil {
			fmt.Fprintf(&builder, format, mangler(param.Name))
		} else {
			fmt.Fprintf(&builder, format, param.Name)
		}
	}
	return builder.String()
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
