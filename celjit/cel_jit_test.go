package celjit

import (
	"reflect"
	"testing"

	"github.com/google/cel-go/cel"
)

var tests = []struct{
	name string
	expr string
	paramNames []string
	paramValues []any
}{
	{"Addition", "a + b + c", []string{"a", "b", "c"}, []any{int64(1), int64(2), int64(3)}},
	{"IntConst", "123", nil, nil},
	{"UintConst", "123u", nil, nil},
	{"DoubleConst", "1.23", nil, nil},
	{"StringConst", "\"foobar\"", nil, nil},
	{"BytesConst", "b\"foobar\"", nil, nil},
}

func TestConformance(t *testing.T) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// CEL.
			envOpts := make([]cel.EnvOption, 0, len(test.paramNames)+2)
			envOpts = append(envOpts,
				cel.EagerlyValidateDeclarations(true),
				cel.ExtendedValidations(),
			)
			for _, paramName := range test.paramNames{
				envOpts = append(envOpts, cel.Variable(paramName, cel.DynType))
			}
			env, err := cel.NewEnv(envOpts...)
			if err != nil {
				t.Errorf("Failed to create CEL env: %v", err)
				return
			}

			ast, iss := env.Compile(test.expr)
			if err := iss.Err(); err != nil {
				t.Errorf("Failed to parse CEL: %v", err)
				return
			}

			prog, err := env.Program(ast)
			if err != nil {
				t.Errorf("Failed to generate CEL program: %v", err)
				return
			}

			celArgs := make(map[string]any, len(test.paramNames))
			for i, paramName := range test.paramNames {
				celArgs[paramName] = test.paramValues[i]
			}

			celResult, _, err := prog.ContextEval(t.Context(), celArgs)
			if err != nil {
				t.Errorf("Failed to execute CEL program: %v", err)
				return
			}
			celResultNative := celResult.Value()

			// JIT.
			jitParameters := make([]Parameter, 0, len(test.paramNames))
			for _, paramName := range test.paramNames {
				jitParameters = append(jitParameters, Parameter{
					Name: paramName,
					Type: cel.DynType,
				})
			}
			fAny, err := Compile(test.expr, Config{
				Parameters: jitParameters,
			})
			if err != nil {
				t.Errorf("Failed to compile JIT: %v", err)
				return
			}

			jitArgs := make([]reflect.Value, 0, len(test.paramNames))
			for _, paramValue := range test.paramValues {
				jitArgs = append(jitArgs, reflect.ValueOf(paramValue))
			}
			resSlice := reflect.ValueOf(fAny).Call(jitArgs)
			if err, _ := resSlice[1].Interface().(error); err != nil {
				t.Errorf("Failed to execute JIT: %v", err)
				return
			}
			jitResult := resSlice[0].Interface()

			// Compare.
			if !reflect.DeepEqual(celResultNative, jitResult) {
				t.Errorf("Results were not the same. CEL produced: %v, type %[1]T; JIT produced: %v, type %[2]T", celResult, jitResult)
			}
		})
	}
}
