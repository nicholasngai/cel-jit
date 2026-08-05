package celjit

import (
	"os"
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
	{"HasInvalid", "has(dyn(1).bar)", nil, nil},
	{"All", "[1, 2, 3].all(x, x == 1)", nil, nil},
	{"AllMap", "{\"foo\": 1, \"bar\": 2}.all(x, x == \"foo\")", nil, nil},
	{"AllError", "[1, 2, {}.error].all(x, x == 1)", nil, nil},
	{"Exists", "[1, 2, 3].exists(x, x == 1)", nil, nil},
	{"ExistsMap", "{\"foo\": 1, \"bar\": 2}.exists(x, x == \"foo\")", nil, nil},
	{"ExistsError", "[1, 2, {}.error].exists(x, x == 1)", nil, nil},
	{"ExistsOne", "[1, 2, 3].exists_one(x, x == 1)", nil, nil},
	{"ExistsOneMap", "{\"foo\": 1, \"bar\": 2}.exists_one(x, x == \"foo\")", nil, nil},
	{"Map", "[1, 2, 3].map(x, x + 1)", nil, nil},
	{"MapMap", "{\"foo\": 1}.map(x, x + \"_test\")", nil, nil},
	{"MapFilter", "[1, 2, 3].map(x, x == 1, x + 1)", nil, nil},
	{"MapFilterMap", "{\"foo\": 1, \"bar\": 2}.map(x, x == \"foo\", x + \"_test\")", nil, nil},
	{"Filter", "[1, 2, 3].filter(x, x == 1)", nil, nil},
	{"FilterMap", "{\"foo\": 1, \"bar\": 2}.filter(x, x == \"foo\")", nil, nil},
	{"ConditionalTrue", "true ? 1 : 2", nil, nil},
	{"ConditionalFalse", "true ? 1 : 2", nil, nil},
	{"ConditionalErrorTrue", "true ? {}.error : 2", nil, nil},
	{"ConditionalErrorFalse", "true ? {}.error : 2", nil, nil},
	{"LogicalAnd", "true && false", nil, nil},
	{"LogicalAndErrorTrue", "{}.error && true", nil, nil},
	{"LogicalAndErrorFalse", "{}.error && false", nil, nil},
	{"LogicalOr", "true || false", nil, nil},
	{"LogicalOrErrorTrue", "{}.error || true", nil, nil},
	{"LogicalOrErrorFalse", "{}.error || false", nil, nil},
	{"Equals", "1 == 1", nil, nil},
	{"EqualsNumeric", "1 == dyn(1.0)", nil, nil},
	{"EqualsList", "[1, 2, 3] == [1, 2, 3]", nil, nil},
	{"EqualsNumericList", "[1, 2, 3] == dyn([1.0, 2.0, 3.0])", nil, nil},
	{"EqualsNestedList", "[[1], [2, 3]] == [[1], [2, 3]]", nil, nil},
	{"EqualsMap", "{1: 2, 3: 4} == {1: 2, 3: 4}", nil, nil},
	{"EqualsMapNumeric", "{1: 2, 3: 4} == dyn({1u: 2.0, 3u: 4.0})", nil, nil},
	{"EqualsTypes", "1 == dyn(\"foobar\")", nil, nil},
	{"NotEquals", "1 != 1", nil, nil},
	{"NotEqualsNumeric", "1 != dyn(1.0)", nil, nil},
	{"NotEqualsList", "[1, 2, 3] != [1, 2, 3]", nil, nil},
	{"NotEqualsNumericList", "[1, 2, 3] != dyn([1.0, 2.0, 3.0])", nil, nil},
	{"NotEqualsNestedList", "[[1], [2, 3]] != [[1], [2, 3]]", nil, nil},
	{"NotEqualsMap", "{1: 2, 3: 4} != {1: 2, 3: 4}", nil, nil},
	{"NotEqualsMapNumeric", "{1: 2, 3: 4} != dyn({1u: 2.0, 3u: 4.0})", nil, nil},
	{"NotEqualsTypes", "1 != dyn(\"foobar\")", nil, nil},
	{"Less", "1 < 2", nil, nil},
	{"LessEquals", "1 <= 2", nil, nil},
	{"Greater", "1 < 2", nil, nil},
	{"GreaterEquals", "1 <= 2", nil, nil},
	{"Add", "1 + 2 + 3", nil, nil},
	{"Subtract", "1 - 2 - 3", nil, nil},
	{"Multiply", "1 * 2 * 3", nil, nil},
	{"Divide", "3 / 2", nil, nil},
	{"Modulo", "1 % 2", nil, nil},
	{"Negate", "-(1)", nil, nil},
	{"Index", "[1, 2, 3][0]", nil, nil},
	{"IndexOutOfRange", "[1, 2, 3][10]", nil, nil},
	{"IndexFloat", "[1, 2, 3][dyn(0.0)]", nil, nil},
	{"IndexFloat2", "[1, 2, 3][dyn(0.5)]", nil, nil},
	{"IndexMap", "{\"foo\": 1, \"bar\": 2}[\"foo\"]", nil, nil},
	{"IndexMapMissing", "{\"foo\": 1, \"bar\": 2}[\"fizzbuzz\"]", nil, nil},
	{"IndexMapNumeric", "{1u: \"foo\", 2u: \"bar\"}[dyn(1)]", nil, nil},
	{"In", "1 in [1, 2, 3]", nil, nil},
	{"InNumeric", "dyn(1) in [1.0, 2.0, 3.0]", nil, nil},
	{"InMap", "\"foo\" in {\"foo\": 1, \"bar\": 2}", nil, nil},
	{"InMapNumeric", "dyn(1) in {1u: \"foo\", 2u: \"bar\"}", nil, nil},
	{"SizeString", "size(\"foobar\")", nil, nil},
	{"SizeStringOverload", "\"foobar\".size()", nil, nil},
	{"SizeBytes", "size(b\"foobar\")", nil, nil},
	{"SizeBytesOverload", "b\"foobar\".size()", nil, nil},
	{"SizeList", "size([1, 2, 3])", nil, nil},
	{"SizeListOverload", "[1, 2, 3].size()", nil, nil},
	{"SizeMap", "size({\"foo\": 1, \"bar\": 2})", nil, nil},
	{"SizeMapOverload", "{\"foo\": 1, \"bar\": 2}.size()", nil, nil},
	{"Contains", "\"foobar\".contains(\"foo\")", nil, nil},
	{"EndsWith", "\"foobar\".endsWith(\"bar\")", nil, nil},
	{"Matches", "matches(\"foobar\", \"foo\")", nil, nil},
	{"MatchesOverload", "\"foobar\".matches(\"foo\")", nil, nil},
	{"StartsWith", "\"foobar\".startsWith(\"foo\")", nil, nil},
	{"GetFullYear", "timestamp(\"2026-08-04T12:34:56Z\").getFullYear()", nil, nil},
	{"GetFullYearTZ", "timestamp(\"2026-08-04T12:34:56Z\").getFullYear(\"America/Los_Angeles\")", nil, nil},
	{"GetMonth", "timestamp(\"2026-08-04T12:34:56Z\").getMonth()", nil, nil},
	{"GetMonthTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMonth(\"America/Los_Angeles\")", nil, nil},
	{"GetDayOfYear", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfYear()", nil, nil},
	{"GetDayOfYearTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfYear(\"America/Los_Angeles\")", nil, nil},
	{"GetDate", "timestamp(\"2026-08-04T12:34:56Z\").getDate()", nil, nil},
	{"GetDateTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDate(\"America/Los_Angeles\")", nil, nil},
	{"GetDayOfMonth", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfMonth()", nil, nil},
	{"GetDayOfMonthTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfMonth(\"America/Los_Angeles\")", nil, nil},
	{"GetDayOfWeek", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfWeek()", nil, nil},
	{"GetDayOfWeekTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfWeek(\"America/Los_Angeles\")", nil, nil},
	{"GetHours", "timestamp(\"2026-08-04T12:34:56Z\").getHours()", nil, nil},
	{"GetHoursTZ", "timestamp(\"2026-08-04T12:34:56Z\").getHours(\"America/Los_Angeles\")", nil, nil},
	{"GetHoursDuration", "duration(\"1h\").getHours()", nil, nil},
	{"GetMinutes", "timestamp(\"2026-08-04T12:34:56Z\").getMinutes()", nil, nil},
	{"GetMinutesTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMinutes(\"America/Los_Angeles\")", nil, nil},
	{"GetMinutesDuration", "duration(\"1h\").getMinutes()", nil, nil},
	{"GetSeconds", "timestamp(\"2026-08-04T12:34:56Z\").getSeconds()", nil, nil},
	{"GetSecondsTZ", "timestamp(\"2026-08-04T12:34:56Z\").getSeconds(\"America/Los_Angeles\")", nil, nil},
	{"GetSecondsDuration", "duration(\"1h\").getSeconds()", nil, nil},
	{"GetMilliseconds", "timestamp(\"2026-08-04T12:34:56Z\").getMilliseconds()", nil, nil},
	{"GetMillisecondsTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMilliseconds(\"America/Los_Angeles\")", nil, nil},
	// Commented out since cel-go is currently non-conformant.
	//{"GetMillisecondsDuration", "duration(\"1h\").getMilliseconds()", nil, nil},
	{"Variable", "x + 1", []string{"x"}, []any{int64(1)}},
	{"VariableWithDot", "x.y + 1", []string{"x.y"}, []any{int64(1)}},
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

func TestMain(m *testing.M) {
	exitCode := m.Run()
	Cleanup()
	os.Exit(exitCode)
}
