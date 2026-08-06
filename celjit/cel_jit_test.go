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
	paramTypes []*cel.Type
	paramValues []any
	returnType *cel.Type
}{
	{"IntConst", "123", nil, nil, nil, cel.IntType},
	{"UintConst", "123u", nil, nil, nil, cel.DynType},
	{"DoubleConst", "1.23", nil, nil, nil, cel.DynType},
	{"StringConst", "\"foobar\"", nil, nil, nil, cel.DynType},
	{"BytesConst", "b\"foobar\"", nil, nil, nil, cel.DynType},
	{"NullLiteral", "null", nil, nil, nil, cel.DynType},
	{"ListLiteral", "[1, 2, 3]", nil, nil, nil, cel.DynType},
	{"MapLiteral", "{\"foo\": 1, \"bar\": 2}", nil, nil, nil, cel.DynType},
	{"Select", "{\"foo\": 1}.foo", nil, nil, nil, cel.DynType},
	{"SelectMissing", "{\"foo\": 1}.bar", nil, nil, nil, cel.DynType},
	{"SelectNull", "{\"foo\": null}.foo", nil, nil, nil, cel.DynType},
	{"SelectInvalid", "dyn(1).foo", nil, nil, nil, cel.DynType},
	{"Has", "has({\"foo\": 1}.foo)", nil, nil, nil, cel.DynType},
	{"HasMissing", "has({\"foo\": 1}.bar)", nil, nil, nil, cel.DynType},
	{"HasNull", "has({\"foo\": null}.foo)", nil, nil, nil, cel.DynType},
	{"HasInvalid", "has(dyn(1).bar)", nil, nil, nil, cel.DynType},
	{"All", "[1, 2, 3].all(x, x == 1)", nil, nil, nil, cel.DynType},
	{"AllMap", "{\"foo\": 1, \"bar\": 2}.all(x, x == \"foo\")", nil, nil, nil, cel.DynType},
	{"AllError", "[1, 2, {}.error].all(x, x == 1)", nil, nil, nil, cel.DynType},
	{"Exists", "[1, 2, 3].exists(x, x == 1)", nil, nil, nil, cel.DynType},
	{"ExistsMap", "{\"foo\": 1, \"bar\": 2}.exists(x, x == \"foo\")", nil, nil, nil, cel.DynType},
	{"ExistsError", "[1, 2, {}.error].exists(x, x == 1)", nil, nil, nil, cel.DynType},
	{"ExistsOne", "[1, 2, 3].exists_one(x, x == 1)", nil, nil, nil, cel.DynType},
	{"ExistsOneMap", "{\"foo\": 1, \"bar\": 2}.exists_one(x, x == \"foo\")", nil, nil, nil, cel.DynType},
	{"Map", "[1, 2, 3].map(x, x + 1)", nil, nil, nil, cel.DynType},
	{"MapMap", "{\"foo\": 1}.map(x, x + \"_test\")", nil, nil, nil, cel.DynType},
	{"MapFilter", "[1, 2, 3].map(x, x == 1, x + 1)", nil, nil, nil, cel.DynType},
	{"MapFilterMap", "{\"foo\": 1, \"bar\": 2}.map(x, x == \"foo\", x + \"_test\")", nil, nil, nil, cel.DynType},
	{"Filter", "[1, 2, 3].filter(x, x == 1)", nil, nil, nil, cel.DynType},
	{"FilterMap", "{\"foo\": 1, \"bar\": 2}.filter(x, x == \"foo\")", nil, nil, nil, cel.DynType},
	{"ConditionalTrue", "true ? 1 : 2", nil, nil, nil, cel.DynType},
	{"ConditionalFalse", "true ? 1 : 2", nil, nil, nil, cel.DynType},
	{"ConditionalErrorTrue", "true ? {}.error : 2", nil, nil, nil, cel.DynType},
	{"ConditionalErrorFalse", "true ? {}.error : 2", nil, nil, nil, cel.DynType},
	{"LogicalAnd", "true && false", nil, nil, nil, cel.DynType},
	{"LogicalAndErrorTrue", "{}.error && true", nil, nil, nil, cel.DynType},
	{"LogicalAndErrorFalse", "{}.error && false", nil, nil, nil, cel.DynType},
	{"LogicalOr", "true || false", nil, nil, nil, cel.DynType},
	{"LogicalOrErrorTrue", "{}.error || true", nil, nil, nil, cel.DynType},
	{"LogicalOrErrorFalse", "{}.error || false", nil, nil, nil, cel.DynType},
	{"Equals", "1 == 1", nil, nil, nil, cel.DynType},
	{"EqualsNumeric", "1 == dyn(1.0)", nil, nil, nil, cel.DynType},
	{"EqualsList", "[1, 2, 3] == [1, 2, 3]", nil, nil, nil, cel.DynType},
	{"EqualsNumericList", "[1, 2, 3] == dyn([1.0, 2.0, 3.0])", nil, nil, nil, cel.DynType},
	{"EqualsNestedList", "[[1], [2, 3]] == [[1], [2, 3]]", nil, nil, nil, cel.DynType},
	{"EqualsMap", "{1: 2, 3: 4} == {1: 2, 3: 4}", nil, nil, nil, cel.DynType},
	{"EqualsMapNumeric", "{1: 2, 3: 4} == dyn({1u: 2.0, 3u: 4.0})", nil, nil, nil, cel.DynType},
	{"EqualsTypes", "1 == dyn(\"foobar\")", nil, nil, nil, cel.DynType},
	{"NotEquals", "1 != 1", nil, nil, nil, cel.DynType},
	{"NotEqualsNumeric", "1 != dyn(1.0)", nil, nil, nil, cel.DynType},
	{"NotEqualsList", "[1, 2, 3] != [1, 2, 3]", nil, nil, nil, cel.DynType},
	{"NotEqualsNumericList", "[1, 2, 3] != dyn([1.0, 2.0, 3.0])", nil, nil, nil, cel.DynType},
	{"NotEqualsNestedList", "[[1], [2, 3]] != [[1], [2, 3]]", nil, nil, nil, cel.DynType},
	{"NotEqualsMap", "{1: 2, 3: 4} != {1: 2, 3: 4}", nil, nil, nil, cel.DynType},
	{"NotEqualsMapNumeric", "{1: 2, 3: 4} != dyn({1u: 2.0, 3u: 4.0})", nil, nil, nil, cel.DynType},
	{"NotEqualsTypes", "1 != dyn(\"foobar\")", nil, nil, nil, cel.DynType},
	{"Less", "1 < 2", nil, nil, nil, cel.DynType},
	{"LessEquals", "1 <= 2", nil, nil, nil, cel.DynType},
	{"Greater", "1 < 2", nil, nil, nil, cel.DynType},
	{"GreaterEquals", "1 <= 2", nil, nil, nil, cel.DynType},
	{"Add", "1 + 2 + 3", nil, nil, nil, cel.DynType},
	{"Subtract", "1 - 2 - 3", nil, nil, nil, cel.DynType},
	{"Multiply", "1 * 2 * 3", nil, nil, nil, cel.DynType},
	{"Divide", "3 / 2", nil, nil, nil, cel.DynType},
	{"Modulo", "1 % 2", nil, nil, nil, cel.DynType},
	{"Negate", "-(1)", nil, nil, nil, cel.DynType},
	{"Index", "[1, 2, 3][0]", nil, nil, nil, cel.DynType},
	{"IndexOutOfRange", "[1, 2, 3][10]", nil, nil, nil, cel.DynType},
	{"IndexFloat", "[1, 2, 3][dyn(0.0)]", nil, nil, nil, cel.DynType},
	{"IndexFloat2", "[1, 2, 3][dyn(0.5)]", nil, nil, nil, cel.DynType},
	{"IndexMap", "{\"foo\": 1, \"bar\": 2}[\"foo\"]", nil, nil, nil, cel.DynType},
	{"IndexMapMissing", "{\"foo\": 1, \"bar\": 2}[\"fizzbuzz\"]", nil, nil, nil, cel.DynType},
	{"IndexMapNumeric", "{1u: \"foo\", 2u: \"bar\"}[dyn(1)]", nil, nil, nil, cel.DynType},
	{"In", "1 in [1, 2, 3]", nil, nil, nil, cel.DynType},
	{"InNumeric", "dyn(1) in [1.0, 2.0, 3.0]", nil, nil, nil, cel.DynType},
	{"InMap", "\"foo\" in {\"foo\": 1, \"bar\": 2}", nil, nil, nil, cel.DynType},
	{"InMapNumeric", "dyn(1) in {1u: \"foo\", 2u: \"bar\"}", nil, nil, nil, cel.DynType},
	{"SizeString", "size(\"foobar\")", nil, nil, nil, cel.DynType},
	{"SizeStringOverload", "\"foobar\".size()", nil, nil, nil, cel.DynType},
	{"SizeBytes", "size(b\"foobar\")", nil, nil, nil, cel.DynType},
	{"SizeBytesOverload", "b\"foobar\".size()", nil, nil, nil, cel.DynType},
	{"SizeList", "size([1, 2, 3])", nil, nil, nil, cel.DynType},
	{"SizeListOverload", "[1, 2, 3].size()", nil, nil, nil, cel.DynType},
	{"SizeMap", "size({\"foo\": 1, \"bar\": 2})", nil, nil, nil, cel.DynType},
	{"SizeMapOverload", "{\"foo\": 1, \"bar\": 2}.size()", nil, nil, nil, cel.DynType},
	{"Contains", "\"foobar\".contains(\"foo\")", nil, nil, nil, cel.DynType},
	{"EndsWith", "\"foobar\".endsWith(\"bar\")", nil, nil, nil, cel.DynType},
	{"Matches", "matches(\"foobar\", \"foo\")", nil, nil, nil, cel.DynType},
	{"MatchesOverload", "\"foobar\".matches(\"foo\")", nil, nil, nil, cel.DynType},
	{"StartsWith", "\"foobar\".startsWith(\"foo\")", nil, nil, nil, cel.DynType},
	{"GetFullYear", "timestamp(\"2026-08-04T12:34:56Z\").getFullYear()", nil, nil, nil, cel.DynType},
	{"GetFullYearTZ", "timestamp(\"2026-08-04T12:34:56Z\").getFullYear(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetMonth", "timestamp(\"2026-08-04T12:34:56Z\").getMonth()", nil, nil, nil, cel.DynType},
	{"GetMonthTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMonth(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetDayOfYear", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfYear()", nil, nil, nil, cel.DynType},
	{"GetDayOfYearTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfYear(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetDate", "timestamp(\"2026-08-04T12:34:56Z\").getDate()", nil, nil, nil, cel.DynType},
	{"GetDateTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDate(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetDayOfMonth", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfMonth()", nil, nil, nil, cel.DynType},
	{"GetDayOfMonthTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfMonth(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetDayOfWeek", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfWeek()", nil, nil, nil, cel.DynType},
	{"GetDayOfWeekTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfWeek(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetHours", "timestamp(\"2026-08-04T12:34:56Z\").getHours()", nil, nil, nil, cel.DynType},
	{"GetHoursTZ", "timestamp(\"2026-08-04T12:34:56Z\").getHours(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetHoursDuration", "duration(\"1h\").getHours()", nil, nil, nil, cel.DynType},
	{"GetMinutes", "timestamp(\"2026-08-04T12:34:56Z\").getMinutes()", nil, nil, nil, cel.DynType},
	{"GetMinutesTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMinutes(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetMinutesDuration", "duration(\"1h\").getMinutes()", nil, nil, nil, cel.DynType},
	{"GetSeconds", "timestamp(\"2026-08-04T12:34:56Z\").getSeconds()", nil, nil, nil, cel.DynType},
	{"GetSecondsTZ", "timestamp(\"2026-08-04T12:34:56Z\").getSeconds(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	{"GetSecondsDuration", "duration(\"1h\").getSeconds()", nil, nil, nil, cel.DynType},
	{"GetMilliseconds", "timestamp(\"2026-08-04T12:34:56Z\").getMilliseconds()", nil, nil, nil, cel.DynType},
	{"GetMillisecondsTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMilliseconds(\"America/Los_Angeles\")", nil, nil, nil, cel.DynType},
	// Commented out since cel-go is currently non-conformant.
	//{"GetMillisecondsDuration", "duration(\"1h\").getMilliseconds()", nil, nil, nil, cel.DynType},
	{"IntFromInt", "int(1)", nil, nil, nil, cel.DynType},
	{"IntFromUint", "int(1u)", nil, nil, nil, cel.DynType},
	{"IntFromUintOutOfRange", "int(0xffffffffffffffffu)", nil, nil, nil, cel.DynType},
	{"IntFromDouble", "int(1.0)", nil, nil, nil, cel.DynType},
	{"IntFromDoubleHalf", "int(1.5)", nil, nil, nil, cel.DynType},
	{"IntFromDoubleOutOfRange1", "int(1e200)", nil, nil, nil, cel.DynType},
	{"IntFromDoubleOutOfRange2", "int(-1e200)", nil, nil, nil, cel.DynType},
	{"IntFromString", "int(\"1\")", nil, nil, nil, cel.DynType},
	{"IntFromStringInvalid", "int(\"foobar\")", nil, nil, nil, cel.DynType},
	{"IntFromTimestamp", "int(timestamp(\"2026-08-04T12:34:56Z\"))", nil, nil, nil, cel.DynType},
	{"UintFromUint", "uint(1u)", nil, nil, nil, cel.DynType},
	{"UintFromInt", "uint(1)", nil, nil, nil, cel.DynType},
	{"UintFromIntOutOfRange", "uint(-1)", nil, nil, nil, cel.DynType},
	{"UintFromDouble", "uint(1.0)", nil, nil, nil, cel.DynType},
	{"UintFromDoubleHalf", "uint(1.5)", nil, nil, nil, cel.DynType},
	{"UintFromDoubleOutOfRange1", "uint(-1.0)", nil, nil, nil, cel.DynType},
	{"UintFromDoubleOutOfRange2", "uint(1e200)", nil, nil, nil, cel.DynType},
	{"UintFromString", "uint(\"1\")", nil, nil, nil, cel.DynType},
	{"UintFromStringInvalid", "uint(\"foobar\")", nil, nil, nil, cel.DynType},
	{"DoubleFromDouble", "double(1.0)", nil, nil, nil, cel.DynType},
	{"DoubleFromInt", "double(1)", nil, nil, nil, cel.DynType},
	{"DoubleFromUint", "double(1u)", nil, nil, nil, cel.DynType},
	{"DoubleFromString", "double(\"1.0\")", nil, nil, nil, cel.DynType},
	{"DoubleFromStringInvalid", "double(\"foobar\")", nil, nil, nil, cel.DynType},
	{"BoolFromBool", "bool(true)", nil, nil, nil, cel.DynType},
	{"BoolFromString", "bool(\"true\")", nil, nil, nil, cel.DynType},
	{"BoolFromStringInvalid", "bool(\"foobar\")", nil, nil, nil, cel.DynType},
	{"StringFromString", "string(\"foobar\")", nil, nil, nil, cel.DynType},
	{"StringFromInt", "string(1)", nil, nil, nil, cel.DynType},
	{"StringFromUint", "string(1u)", nil, nil, nil, cel.DynType},
	{"StringFromFloat", "string(1.0)", nil, nil, nil, cel.DynType},
	{"StringFromBool", "string(true)", nil, nil, nil, cel.DynType},
	{"StringFromBytes", "string(b\"foobar\")", nil, nil, nil, cel.DynType},
	{"StringFromTimestamp", "string(timestamp(\"2026-08-04T12:34:56Z\"))", nil, nil, nil, cel.DynType},
	{"StringFromDuration", "string(duration(\"1h\"))", nil, nil, nil, cel.DynType},
	{"StringFromDurationFractional", "string(duration(\"60.1s\"))", nil, nil, nil, cel.DynType},
	{"BytesFromBytes", "bytes(b\"foobar\")", nil, nil, nil, cel.DynType},
	{"BytesFromString", "bytes(\"foobar\")", nil, nil, nil, cel.DynType},
	{"TimestampFromTimestamp", "timestamp(timestamp(\"2026-08-04T12:34:56Z\"))", nil, nil, nil, cel.DynType},
	{"TimestampFromString", "timestamp(\"2026-08-04T12:34:56Z\")", nil, nil, nil, cel.DynType},
	{"DurationFromDuration", "duration(duration(\"1s\"))", nil, nil, nil, cel.DynType},
	{"DurationFromString", "duration(\"1s\")", nil, nil, nil, cel.DynType},
	{"Dyn", "dyn(1)", nil, nil, nil, cel.DynType},
	{"Variable", "x + 1", []string{"x"}, []*cel.Type{cel.IntType}, []any{int64(1)}, cel.DynType},
	{"VariableWithDot", "x.y + 1", []string{"x.y"}, []*cel.Type{cel.IntType}, []any{int64(1)}, cel.DynType},
}

func TestConformance(t *testing.T) {
	t.Parallel()

	// Compile JIT.
	jitFuncs, err := compileJITTests()
	if err != nil {
		t.Errorf("Failed to compile JIT: %v", err)
		return
	}

	for i, test := range tests {
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
			jitFunc := jitFuncs[i]
			jitParameters := make([]Parameter, 0, len(test.paramNames))
			for _, paramName := range test.paramNames {
				jitParameters = append(jitParameters, Parameter{
					Name: paramName,
					Type: cel.DynType,
				})
			}

			jitArgs := make([]reflect.Value, 0, len(test.paramNames))
			for _, paramValue := range test.paramValues {
				jitArgs = append(jitArgs, reflect.ValueOf(paramValue))
			}
			resSlice := reflect.ValueOf(jitFunc).Call(jitArgs)
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
				_, _, _ = prog.ContextEval(b.Context(), celArgs)
			}
		})
	}
}

func BenchmarkJIT(b *testing.B) {
	// Compile JIT.
	jitFuncs, err := compileJITTests()
	if err != nil {
		b.Errorf("Failed to compile JIT: %v", err)
		return
	}

	for i, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			jitFunc := jitFuncs[i]

			jitArgs := make([]reflect.Value, 0, len(test.paramNames))
			for _, paramValue := range test.paramValues {
				jitArgs = append(jitArgs, reflect.ValueOf(paramValue))
			}

			switch f := jitFunc.(type) {
			case func() (any, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (int64, error):
				for b.Loop() {
					_, _ = f()
				}
			case func(int64) (int64, error):
				param0 := test.paramValues[0].(int64)
				for b.Loop() {
					_, _ = f(param0)
				}
			default:
				b.Errorf("Unknown JIT function type for benchmarking: %T", jitFunc)
				return
			}
		})
	}
}

func compileJITTests() ([]any, error) {
	// Compile JIT.
	exprConfigs := make([]ExprConfig, 0, len(tests))
	for _, test := range tests {
		params := make([]Parameter, 0, len(test.paramNames))
		for i, paramName := range test.paramNames {
			paramType := test.paramTypes[i]
			params = append(params, Parameter{
				Name: paramName,
				Type: paramType,
			})
		}
		exprConfigs = append(exprConfigs, ExprConfig{
			Expr: test.expr,
			Parameters: params,
			ReturnType: test.returnType,
		})
	}
	return Compile(Config{
		Exprs: exprConfigs,
	})
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	Cleanup()
	os.Exit(exitCode)
}
