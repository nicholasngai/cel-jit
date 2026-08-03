package main

import (
	"testing"

	"github.com/google/cel-go/cel"

	"github.com/nicholasngai/cel-jit/celjit"
)

func BenchmarkJIT(b *testing.B) {
	fAny, err := celjit.Compile(`a + b == c`, celjit.Config{
		Parameters: []celjit.Parameter{
			{
				Name: "a",
				Type: cel.IntType,
			},
			{
				Name: "b",
				Type: cel.IntType,
			},
			{
				Name: "c",
				Type: cel.IntType,
			},
		},
	})
	if err != nil {
		b.Errorf("Failed to compile JIT: %v", err)
		return
	}
	f := fAny.(func(a any, b any, c any) (any, error))

	for b.Loop() {
		if _, err := f(int64(1), int64(2), int64(3)); err != nil {
			b.Errorf("Failed to execute: %v", err)
			return
		}
	}
}

func BenchmarkInterp(b *testing.B) {
	env, err := cel.NewEnv(
		cel.EagerlyValidateDeclarations(true),
		cel.ExtendedValidations(),
		cel.Variable("a", cel.IntType),
		cel.Variable("b", cel.IntType),
		cel.Variable("c", cel.IntType),
	)
	if err != nil {
		b.Errorf("Failed to create CEL env: %v", err)
		return
	}

	ast, iss := env.Compile(`a + b == c`)
	if err := iss.Err(); err != nil {
		b.Errorf("Failed to compile CEL expression: %v", err)
		return
	}

	prog, err := env.Program(ast)
	if err != nil {
		b.Errorf("Failed to generate CEL program: %v", err)
		return
	}

	for b.Loop() {
		if _, _, err := prog.Eval(map[string]any{
			"a": int64(1),
			"b": int64(2),
			"c": int64(3),
		}); err != nil {
			b.Errorf("Failed to execute: %v", err)
			return
		}
	}
}
