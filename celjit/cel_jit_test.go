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
	{"NullLiteral", "null", nil, nil},
	{"ListLiteral", "[1, 2, 3]", nil, nil},
	{"MapLiteral", "{\"foo\": 1, \"bar\": 2}", nil, nil},
	{"Select", "{\"foo\": 1}.foo", nil, nil},
	{"SelectMissing", "{\"foo\": 1}.bar", nil, nil},
	{"SelectNull", "{\"foo\": null}.foo", nil, nil},
	{"SelectInvalid", "dyn(1).foo", nil, nil},
	{"Has", "has({\"foo\": 1}.foo)", nil, nil},
	{"HasMissing", "has({\"foo\": 1}.bar)", nil, nil},
	{"HasNull", "has({\"foo\": null}.foo)", nil, nil},
	{"All", "[1, 2, 3].all(x, x == 1)", nil, nil},
	{"AllError", "[1, 2, {}.error].all(x, x == 1)", nil, nil},
	{"Exists", "[1, 2, 3].exists(x, x == 1)", nil, nil},
	{"ExistsError", "[1, 2, {}.error].exists(x, x == 1)", nil, nil},
	{"ExistsOne", "[1, 2, 3].exists_one(x, x == 1)", nil, nil},
	{"Map", "[1, 2, 3].map(x, x + 1)", nil, nil},
	{"MapFilter", "[1, 2, 3].map(x, x == 1, x + 1)", nil, nil},
	{"Filter", "[1, 2, 3].filter(x, x == 1)", nil, nil},
	{"HasInvalid", "has(dyn(1).bar)", nil, nil},
	{"Equality", "1 == 1", nil, nil},
	{"NumericEquality", "1 == dyn(1.0)", nil, nil},
	{"ListEquality", "[1, 2, 3] == [1, 2, 3]", nil, nil},
	{"ListNumericEquality", "[1, 2, 3] == dyn([1.0, 2.0, 3.0])", nil, nil},
	{"NestedListEquality", "[[1], [2, 3]] == [[1], [2, 3]]", nil, nil},
	{"MapEquality", "{1: 2, 3: 4} == {1: 2, 3: 4}", nil, nil},
	{"MapNumericEquality", "{1: 2, 3: 4} == dyn({1u: 2.0, 3u: 4.0})", nil, nil},
}

func TestConformance(t *testing.T) {
	t.Parallel()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

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

			celResult, _, celErr := prog.ContextEval(t.Context(), celArgs)
			var celResultNative any
			if celErr == nil {
				r, err := celResult.ConvertToNative(reflect.TypeFor[any]())
				if err != nil {
					t.Errorf("Failed to convert CEL result to native: %v", err)
					return
				}
				celResultNative = r
			}

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
			jitErr, _ := resSlice[1].Interface().(error)
			var jitResult any
			if jitErr == nil {
				jitResult = resSlice[0].Interface()
			}

			// Compare.
			if celErr == nil && jitErr == nil {
				if !reflect.DeepEqual(celResultNative, jitResult) && !(celResult.Type() == cel.NullType && jitResult == nil) {
					t.Errorf("Results were not the same. CEL produced: %v, type %[1]T; JIT produced: %v, type %[2]T", celResultNative, jitResult)
				}
			} else if celErr == nil {
				t.Errorf("JIT returned an error when CEL did not return an error: %v", jitErr)
			} else if jitErr == nil {
				t.Errorf("JIT returned no error when CEL returned an error: %v", celErr)
			}
		})
	}
}

func BenchmarkCEL(b *testing.B) {
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
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
				b.Errorf("Failed to create CEL env: %v", err)
				return
			}

			ast, iss := env.Compile(test.expr)
			if err := iss.Err(); err != nil {
				b.Errorf("Failed to parse CEL: %v", err)
				return
			}

			prog, err := env.Program(ast)
			if err != nil {
				b.Errorf("Failed to generate CEL program: %v", err)
				return
			}

			celArgs := make(map[string]any, len(test.paramNames))
			for i, paramName := range test.paramNames {
				celArgs[paramName] = test.paramValues[i]
			}

			for b.Loop() {
				if _, _, err := prog.ContextEval(b.Context(), celArgs); err != nil {
					b.Errorf("Failed to execute CEL program: %v", err)
					return
				}
			}
		})
	}
}

func BenchmarkJIT(b *testing.B) {
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
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
				b.Errorf("Failed to compile JIT: %v", err)
				return
			}

			jitArgs := make([]reflect.Value, 0, len(test.paramNames))
			for _, paramValue := range test.paramValues {
				jitArgs = append(jitArgs, reflect.ValueOf(paramValue))
			}

			switch f := fAny.(type) {
			case func() (any, error):
				for b.Loop() {
					if _, err := f(); err != nil {
						b.Errorf("Failed to execute JIT: %v", err)
						return
					}
				}
			case func(any, any, any) (any, error):
				for b.Loop() {
					if _, err := f(test.paramValues[0], test.paramValues[1], test.paramValues[2]); err != nil {
						b.Errorf("Failed to execute JIT: %v", err)
						return
					}
				}
			default:
				b.Errorf("Unknown JIT function type for benchmarking: %T", fAny)
				return
			}
		})
	}
}
