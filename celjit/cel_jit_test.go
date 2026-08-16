package celjit

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

var tests = []struct {
	name        string
	expr        string
	paramNames  []string
	paramTypes  []*cel.Type
	paramValues []any
	returnType  *cel.Type
}{
	{"IntConst", "123", nil, nil, nil, cel.IntType},
	{"UintConst", "123u", nil, nil, nil, cel.UintType},
	{"DoubleConst", "1.23", nil, nil, nil, cel.DoubleType},
	{"StringConst", "\"foobar\"", nil, nil, nil, cel.StringType},
	{"BytesConst", "b\"foobar\"", nil, nil, nil, cel.BytesType},
	{"NullLiteral", "null", nil, nil, nil, cel.NullType},
	{"ListLiteral", "[1, 2, 3]", nil, nil, nil, cel.ListType(cel.IntType)},
	{"ListLiteralNested", "[[], [[], []]]", nil, nil, nil, cel.ListType(cel.ListType(cel.ListType(cel.DynType)))},
	{"MapLiteral", "{\"foo\": 1, \"bar\": 2}", nil, nil, nil, cel.MapType(cel.StringType, cel.IntType)},
	{"Select", "{\"foo\": 1}.foo", nil, nil, nil, cel.IntType},
	{"SelectMissing", "{\"foo\": 1}.bar", nil, nil, nil, cel.IntType},
	{"SelectNull", "{\"foo\": null}.foo", nil, nil, nil, cel.NullType},
	{"SelectInvalid", "dyn(1).foo", nil, nil, nil, cel.DynType},
	{"Has", "has({\"foo\": 1}.foo)", nil, nil, nil, cel.BoolType},
	{"HasMissing", "has({\"foo\": 1}.bar)", nil, nil, nil, cel.BoolType},
	{"HasNull", "has({\"foo\": null}.foo)", nil, nil, nil, cel.BoolType},
	{"HasInvalid", "has(dyn(1).bar)", nil, nil, nil, cel.BoolType},
	{"All", "[1, 2, 3].all(x, x == 1)", nil, nil, nil, cel.BoolType},
	{"AllMap", "{\"foo\": 1, \"bar\": 2}.all(x, x == \"foo\")", nil, nil, nil, cel.BoolType},
	{"AllError", "[1, 2, 1 / 0].all(x, x == 1)", nil, nil, nil, cel.BoolType},
	{"Exists", "[1, 2, 3].exists(x, x == 1)", nil, nil, nil, cel.BoolType},
	{"ExistsMap", "{\"foo\": 1, \"bar\": 2}.exists(x, x == \"foo\")", nil, nil, nil, cel.BoolType},
	{"ExistsError", "[1, 2, 1 / 0].exists(x, x == 1)", nil, nil, nil, cel.BoolType},
	{"ExistsOne", "[1, 2, 3].exists_one(x, x == 1)", nil, nil, nil, cel.BoolType},
	{"ExistsOneMap", "{\"foo\": 1, \"bar\": 2}.exists_one(x, x == \"foo\")", nil, nil, nil, cel.BoolType},
	{"Map", "[1, 2, 3].map(x, x + 1)", nil, nil, nil, cel.ListType(cel.IntType)},
	{"MapMap", "{\"foo\": 1}.map(x, x + \"_test\")", nil, nil, nil, cel.ListType(cel.StringType)},
	{"MapFilter", "[1, 2, 3].map(x, x == 1, x + 1)", nil, nil, nil, cel.ListType(cel.IntType)},
	{"MapFilterMap", "{\"foo\": 1, \"bar\": 2}.map(x, x == \"foo\", x + \"_test\")", nil, nil, nil, cel.ListType(cel.StringType)},
	{"Filter", "[1, 2, 3].filter(x, x == 1)", nil, nil, nil, cel.ListType(cel.IntType)},
	{"FilterMap", "{\"foo\": 1, \"bar\": 2}.filter(x, x == \"foo\")", nil, nil, nil, cel.ListType(cel.StringType)},
	{"ConditionalTrue", "true ? 1 : 2", nil, nil, nil, cel.IntType},
	{"ConditionalFalse", "false ? 1 : 2", nil, nil, nil, cel.IntType},
	{"ConditionalErrorTrue", "true ? dyn(1 / 0) : 2", nil, nil, nil, cel.IntType},
	{"ConditionalErrorFalse", "true ? dyn(1 / 0) : 2", nil, nil, nil, cel.IntType},
	{"LogicalAnd", "true && false", nil, nil, nil, cel.BoolType},
	{"LogicalAndErrorTrue", "dyn(1 / 0) && true", nil, nil, nil, cel.BoolType},
	{"LogicalAndErrorFalse", "dyn(1 / 0) && false", nil, nil, nil, cel.BoolType},
	{"LogicalAndTrueError", "true && dyn(1 / 0)", nil, nil, nil, cel.BoolType},
	{"LogicalAndFalseError", "false && dyn(1 / 0)", nil, nil, nil, cel.BoolType},
	{"LogicalOr", "true || false", nil, nil, nil, cel.BoolType},
	{"LogicalOrErrorTrue", "dyn(1 / 0) || true", nil, nil, nil, cel.BoolType},
	{"LogicalOrErrorFalse", "dyn(1 / 0) || false", nil, nil, nil, cel.BoolType},
	{"LogicalOrTrueError", "true || dyn(1 / 0)", nil, nil, nil, cel.BoolType},
	{"LogicalOrFalseError", "false || dyn(1 / 0)", nil, nil, nil, cel.BoolType},
	{"LogicalNot", "!false", nil, nil, nil, cel.BoolType},
	{"Equals", "1 == 1", nil, nil, nil, cel.BoolType},
	{"EqualsNumeric", "1 == dyn(1.0)", nil, nil, nil, cel.BoolType},
	{"EqualsList", "[1, 2, 3] == [1, 2, 3]", nil, nil, nil, cel.BoolType},
	{"EqualsNumericList", "[1, 2, 3] == dyn([1.0, 2.0, 3.0])", nil, nil, nil, cel.BoolType},
	{"EqualsNestedList", "[[1], [2, 3]] == [[1], [2, 3]]", nil, nil, nil, cel.BoolType},
	{"EqualsMap", "{1: 2, 3: 4} == {1: 2, 3: 4}", nil, nil, nil, cel.BoolType},
	{"EqualsMapNumeric", "{1: 2, 3: 4} == dyn({1u: 2.0, 3u: 4.0})", nil, nil, nil, cel.BoolType},
	{"EqualsTypes", "1 == dyn(\"foobar\")", nil, nil, nil, cel.BoolType},
	{"NotEquals", "1 != 1", nil, nil, nil, cel.BoolType},
	{"NotEqualsNumeric", "1 != dyn(1.0)", nil, nil, nil, cel.BoolType},
	{"NotEqualsList", "[1, 2, 3] != [1, 2, 3]", nil, nil, nil, cel.BoolType},
	{"NotEqualsNumericList", "[1, 2, 3] != dyn([1.0, 2.0, 3.0])", nil, nil, nil, cel.BoolType},
	{"NotEqualsNestedList", "[[1], [2, 3]] != [[1], [2, 3]]", nil, nil, nil, cel.BoolType},
	{"NotEqualsMap", "{1: 2, 3: 4} != {1: 2, 3: 4}", nil, nil, nil, cel.BoolType},
	{"NotEqualsMapNumeric", "{1: 2, 3: 4} != dyn({1u: 2.0, 3u: 4.0})", nil, nil, nil, cel.BoolType},
	{"NotEqualsTypes", "1 != dyn(\"foobar\")", nil, nil, nil, cel.BoolType},
	{"Less", "1 < 2", nil, nil, nil, cel.BoolType},
	{"LessEquals", "1 <= 2", nil, nil, nil, cel.BoolType},
	{"Greater", "1 < 2", nil, nil, nil, cel.BoolType},
	{"GreaterEquals", "1 <= 2", nil, nil, nil, cel.BoolType},
	{"Add", "1 + 2 + 3", nil, nil, nil, cel.IntType},
	{"Subtract", "1 - 2 - 3", nil, nil, nil, cel.IntType},
	{"Multiply", "1 * 2 * 3", nil, nil, nil, cel.IntType},
	{"Divide", "3 / 2", nil, nil, nil, cel.IntType},
	{"DivideZero", "1 / 0", nil, nil, nil, cel.IntType},
	{"DivideZeroDouble", "1.0 / 0.0", nil, nil, nil, cel.DoubleType},
	{"Modulo", "1 % 2", nil, nil, nil, cel.IntType},
	{"ModuloZero", "1 % 0", nil, nil, nil, cel.IntType},
	{"Negate", "-(1)", nil, nil, nil, cel.IntType},
	{"Index", "[1, 2, 3][0]", nil, nil, nil, cel.IntType},
	{"IndexOutOfRange", "[1, 2, 3][10]", nil, nil, nil, cel.IntType},
	{"IndexFloat", "[1, 2, 3][dyn(0.0)]", nil, nil, nil, cel.IntType},
	{"IndexFloat2", "[1, 2, 3][dyn(0.5)]", nil, nil, nil, cel.IntType},
	{"IndexMap", "{\"foo\": 1, \"bar\": 2}[\"foo\"]", nil, nil, nil, cel.IntType},
	{"IndexMapMissing", "{\"foo\": 1, \"bar\": 2}[\"fizzbuzz\"]", nil, nil, nil, cel.IntType},
	{"IndexMapNumeric", "{1u: \"foo\", 2u: \"bar\"}[dyn(1)]", nil, nil, nil, cel.StringType},
	{"IndexMapNumericDouble", "{1.0: \"foo\", 2.0: \"bar\"}[dyn(1)]", nil, nil, nil, cel.StringType},
	{"IndexMapNumericDyn", "{dyn(1u): \"foo\", dyn(2u): \"bar\"}[1]", nil, nil, nil, cel.StringType},
	{"In", "1 in [1, 2, 3]", nil, nil, nil, cel.BoolType},
	{"InNumeric", "dyn(1) in [1.0, 2.0, 3.0]", nil, nil, nil, cel.BoolType},
	{"InNumericDyn", "1 in [dyn(1.0), dyn(2.0), dyn(3.0)]", nil, nil, nil, cel.BoolType},
	{"InMap", "\"foo\" in {\"foo\": 1, \"bar\": 2}", nil, nil, nil, cel.BoolType},
	{"InMapNumeric", "dyn(1) in {1u: \"foo\", 2u: \"bar\"}", nil, nil, nil, cel.BoolType},
	{"InMapNumericDouble", "dyn(1) in {1.0: \"foo\", 2.0: \"bar\"}", nil, nil, nil, cel.BoolType},
	{"InMapNumericDyn", "1 in {dyn(1u): \"foo\", dyn(2u): \"bar\", dyn(3u): \"fizzbuzz\"}", nil, nil, nil, cel.BoolType},
	{"SizeString", "size(\"foobar\")", nil, nil, nil, cel.IntType},
	{"SizeStringOverload", "\"foobar\".size()", nil, nil, nil, cel.IntType},
	{"SizeBytes", "size(b\"foobar\")", nil, nil, nil, cel.IntType},
	{"SizeBytesOverload", "b\"foobar\".size()", nil, nil, nil, cel.IntType},
	{"SizeList", "size([1, 2, 3])", nil, nil, nil, cel.IntType},
	{"SizeListOverload", "[1, 2, 3].size()", nil, nil, nil, cel.IntType},
	{"SizeMap", "size({\"foo\": 1, \"bar\": 2})", nil, nil, nil, cel.IntType},
	{"SizeMapOverload", "{\"foo\": 1, \"bar\": 2}.size()", nil, nil, nil, cel.IntType},
	{"Contains", "\"foobar\".contains(\"foo\")", nil, nil, nil, cel.BoolType},
	{"EndsWith", "\"foobar\".endsWith(\"bar\")", nil, nil, nil, cel.BoolType},
	{"Matches", "matches(\"foobar\", \"foo\")", nil, nil, nil, cel.BoolType},
	{"MatchesOverload", "\"foobar\".matches(\"foo\")", nil, nil, nil, cel.BoolType},
	{"StartsWith", "\"foobar\".startsWith(\"foo\")", nil, nil, nil, cel.BoolType},
	{"GetFullYear", "timestamp(\"2026-08-04T12:34:56Z\").getFullYear()", nil, nil, nil, cel.IntType},
	{"GetFullYearTZ", "timestamp(\"2026-08-04T12:34:56Z\").getFullYear(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetMonth", "timestamp(\"2026-08-04T12:34:56Z\").getMonth()", nil, nil, nil, cel.IntType},
	{"GetMonthTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMonth(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetDayOfYear", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfYear()", nil, nil, nil, cel.IntType},
	{"GetDayOfYearTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfYear(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetDate", "timestamp(\"2026-08-04T12:34:56Z\").getDate()", nil, nil, nil, cel.IntType},
	{"GetDateTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDate(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetDayOfMonth", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfMonth()", nil, nil, nil, cel.IntType},
	{"GetDayOfMonthTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfMonth(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetDayOfWeek", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfWeek()", nil, nil, nil, cel.IntType},
	{"GetDayOfWeekTZ", "timestamp(\"2026-08-04T12:34:56Z\").getDayOfWeek(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetHours", "timestamp(\"2026-08-04T12:34:56Z\").getHours()", nil, nil, nil, cel.IntType},
	{"GetHoursTZ", "timestamp(\"2026-08-04T12:34:56Z\").getHours(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetHoursDuration", "duration(\"1h\").getHours()", nil, nil, nil, cel.IntType},
	{"GetMinutes", "timestamp(\"2026-08-04T12:34:56Z\").getMinutes()", nil, nil, nil, cel.IntType},
	{"GetMinutesTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMinutes(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetMinutesDuration", "duration(\"1h\").getMinutes()", nil, nil, nil, cel.IntType},
	{"GetSeconds", "timestamp(\"2026-08-04T12:34:56Z\").getSeconds()", nil, nil, nil, cel.IntType},
	{"GetSecondsTZ", "timestamp(\"2026-08-04T12:34:56Z\").getSeconds(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	{"GetSecondsDuration", "duration(\"1h\").getSeconds()", nil, nil, nil, cel.IntType},
	{"GetMilliseconds", "timestamp(\"2026-08-04T12:34:56Z\").getMilliseconds()", nil, nil, nil, cel.IntType},
	{"GetMillisecondsTZ", "timestamp(\"2026-08-04T12:34:56Z\").getMilliseconds(\"America/Los_Angeles\")", nil, nil, nil, cel.IntType},
	// Commented out since cel-go is currently non-conformant.
	//{"GetMillisecondsDuration", "duration(\"1h\").getMilliseconds()", nil, nil, nil, cel.IntType},
	{"IntFromInt", "int(1)", nil, nil, nil, cel.IntType},
	{"IntFromUint", "int(1u)", nil, nil, nil, cel.IntType},
	{"IntFromUintOutOfRange", "int(0xffffffffffffffffu)", nil, nil, nil, cel.IntType},
	{"IntFromDouble", "int(1.0)", nil, nil, nil, cel.IntType},
	{"IntFromDoubleHalf", "int(1.5)", nil, nil, nil, cel.IntType},
	{"IntFromDoubleOutOfRange1", "int(1e200)", nil, nil, nil, cel.IntType},
	{"IntFromDoubleOutOfRange2", "int(-1e200)", nil, nil, nil, cel.IntType},
	{"IntFromString", "int(\"1\")", nil, nil, nil, cel.IntType},
	{"IntFromStringInvalid", "int(\"foobar\")", nil, nil, nil, cel.IntType},
	{"IntFromTimestamp", "int(timestamp(\"2026-08-04T12:34:56Z\"))", nil, nil, nil, cel.IntType},
	{"UintFromUint", "uint(1u)", nil, nil, nil, cel.UintType},
	{"UintFromInt", "uint(1)", nil, nil, nil, cel.UintType},
	{"UintFromIntOutOfRange", "uint(-1)", nil, nil, nil, cel.UintType},
	{"UintFromDouble", "uint(1.0)", nil, nil, nil, cel.UintType},
	{"UintFromDoubleHalf", "uint(1.5)", nil, nil, nil, cel.UintType},
	{"UintFromDoubleOutOfRange1", "uint(-1.0)", nil, nil, nil, cel.UintType},
	{"UintFromDoubleOutOfRange2", "uint(1e200)", nil, nil, nil, cel.UintType},
	{"UintFromString", "uint(\"1\")", nil, nil, nil, cel.UintType},
	{"UintFromStringInvalid", "uint(\"foobar\")", nil, nil, nil, cel.UintType},
	{"DoubleFromDouble", "double(1.0)", nil, nil, nil, cel.DoubleType},
	{"DoubleFromInt", "double(1)", nil, nil, nil, cel.DoubleType},
	{"DoubleFromUint", "double(1u)", nil, nil, nil, cel.DoubleType},
	{"DoubleFromString", "double(\"1.0\")", nil, nil, nil, cel.DoubleType},
	{"DoubleFromStringInvalid", "double(\"foobar\")", nil, nil, nil, cel.DoubleType},
	{"BoolFromBool", "bool(true)", nil, nil, nil, cel.BoolType},
	{"BoolFromString", "bool(\"true\")", nil, nil, nil, cel.BoolType},
	{"BoolFromStringInvalid", "bool(\"foobar\")", nil, nil, nil, cel.BoolType},
	{"StringFromString", "string(\"foobar\")", nil, nil, nil, cel.StringType},
	{"StringFromInt", "string(1)", nil, nil, nil, cel.StringType},
	{"StringFromUint", "string(1u)", nil, nil, nil, cel.StringType},
	{"StringFromFloat", "string(1.0)", nil, nil, nil, cel.StringType},
	{"StringFromBool", "string(true)", nil, nil, nil, cel.StringType},
	{"StringFromBytes", "string(b\"foobar\")", nil, nil, nil, cel.StringType},
	{"StringFromTimestamp", "string(timestamp(\"2026-08-04T12:34:56Z\"))", nil, nil, nil, cel.StringType},
	{"StringFromDuration", "string(duration(\"1h\"))", nil, nil, nil, cel.StringType},
	{"StringFromDurationFractional", "string(duration(\"60.1s\"))", nil, nil, nil, cel.StringType},
	{"BytesFromBytes", "bytes(b\"foobar\")", nil, nil, nil, cel.BytesType},
	{"BytesFromString", "bytes(\"foobar\")", nil, nil, nil, cel.BytesType},
	{"TimestampFromTimestamp", "timestamp(timestamp(\"2026-08-04T12:34:56Z\"))", nil, nil, nil, cel.TimestampType},
	{"TimestampFromString", "timestamp(\"2026-08-04T12:34:56Z\")", nil, nil, nil, cel.TimestampType},
	{"DurationFromDuration", "duration(duration(\"1s\"))", nil, nil, nil, cel.DurationType},
	{"DurationFromString", "duration(\"1s\")", nil, nil, nil, cel.DurationType},
	{"Dyn", "dyn(1)", nil, nil, nil, cel.DynType},
	{"Variable", "x + 1", []string{"x"}, []*cel.Type{cel.IntType}, []any{int64(1)}, cel.IntType},
	{"VariableWithDot", "x.y + 1", []string{"x.y"}, []*cel.Type{cel.IntType}, []any{int64(1)}, cel.IntType},
	{"CustomFunction", "x.addOne()", []string{"x"}, []*cel.Type{cel.IntType}, []any{int64(1)}, cel.IntType},
	{"CustomFunctionError", "x.addOneError(false)", []string{"x"}, []*cel.Type{cel.IntType}, []any{int64(1)}, cel.IntType},
	{"CustomFunctionError2", "x.addOneError(true)", []string{"x"}, []*cel.Type{cel.IntType}, []any{int64(1)}, cel.IntType},
}

func TestConformance(t *testing.T) {
	t.Parallel()

	// Compile JIT.
	jitFuncs, err := compileJITTests(t)
	if err != nil {
		t.Errorf("Failed to compile JIT: %v", err)
		return
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// CEL.
			envOpts := make([]cel.EnvOption, 0, len(test.paramNames))
			for j, paramName := range test.paramNames {
				envOpts = append(envOpts, cel.Variable(paramName, test.paramTypes[j]))
			}
			env, err := makeCELEnv(envOpts...)
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
				if celResult.Type() == cel.NullType && (jitResult == struct{}{}) {
					return
				}

				celResultNative, err := celResult.ConvertToNative(reflect.TypeOf(jitResult))
				if err != nil {
					t.Errorf("Failed to convert CEL result to native: %v", err)
					return
				}

				if !reflect.DeepEqual(celResultNative, jitResult) {
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
			envOpts := make([]cel.EnvOption, 0, len(test.paramNames))
			for _, paramName := range test.paramNames {
				envOpts = append(envOpts, cel.Variable(paramName, cel.DynType))
			}
			env, err := makeCELEnv(envOpts...)
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
	jitFuncs, err := compileJITTests(b)
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
			case func() (uint64, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (float64, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (bool, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (string, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() ([]byte, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() ([]int64, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() ([]string, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() ([][][]any, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (map[string]int64, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (time.Time, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (time.Duration, error):
				for b.Loop() {
					_, _ = f()
				}
			case func() (struct{}, error):
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

func makeCELEnv(opts ...cel.EnvOption) (*cel.Env, error) {
	envOpts := make([]cel.EnvOption, 0, len(opts)+4)
	envOpts = append(envOpts,
		cel.EagerlyValidateDeclarations(true),
		cel.ExtendedValidations(),
		cel.Function("addOne",
			cel.MemberOverload("int_add_one", []*cel.Type{cel.IntType}, cel.IntType, cel.UnaryBinding(func(a ref.Val) ref.Val {
				return types.Int(a.Value().(int64) + 1)
			})),
		),
		cel.Function("addOneError",
			cel.MemberOverload("int_bool_add_one_error", []*cel.Type{cel.IntType, cel.BoolType}, cel.IntType, cel.BinaryBinding(func(a, b ref.Val) ref.Val {
				if b.Value().(bool) {
					return types.NewErr("testing error")
				}
				return types.Int(a.Value().(int64) + 1)
			})),
		),
	)
	envOpts = append(envOpts, opts...)

	return cel.NewEnv(envOpts...)
}

func compileJITTests(tb testing.TB) ([]any, error) {
	tb.Helper()

	// Make env.
	env, err := NewEnv(EnvConfig{
		Functions: map[string]Function{
			"addOne": {
				Overloads: map[string]FunctionOverload{
					"int_add_one": {
						IsMemberOverload: true,
						ParameterTypes:   []*cel.Type{cel.IntType},
						ReturnType:       cel.IntType,
						Implementation:   func(a int64) int64 { return a + 1 },
					},
				},
			},
			"addOneError": {
				Overloads: map[string]FunctionOverload{
					"int_bool_add_one_error": {
						IsMemberOverload: true,
						ParameterTypes:   []*cel.Type{cel.IntType, cel.BoolType},
						ReturnType:       cel.IntType,
						Implementation: func(a int64, b bool) (int64, error) {
							if b {
								return 0, errors.New("testing error")
							}
							return a + 1, nil
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("new env: %w", err)
	}
	tb.Cleanup(func() {
		_ = env.Cleanup()
	})

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
			Expr:       test.expr,
			Parameters: params,
			ReturnType: test.returnType,
		})
	}
	return env.Compile(CompileConfig{
		Exprs: exprConfigs,
	})
}
