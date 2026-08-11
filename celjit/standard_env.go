package celjit

import "github.com/google/cel-go/common/overloads"

// makeStandardEnv returns the standard set of functions in a CEL environment
// according to the CEL spec
// https://github.com/cel-expr/cel-spec/blob/master/doc/langdef.md.
//
// This does no include operators, since operators can't be overloaded by the
// user.
func makeStandardEnv() map[string]envFunction {
	return map[string]envFunction{
		overloads.Size: {
			dynRuntimeName: "runtime.Size",
			maxArguments:   1,
		},
		overloads.EndsWith: {
			dynRuntimeName: "runtime.EndsWith",
			maxArguments:   2,
		},
		overloads.Matches: {
			dynRuntimeName: "runtime.Matches",
			maxArguments:   2,
		},
		overloads.StartsWith: {
			dynRuntimeName: "runtime.StartsWith",
			maxArguments:   2,
		},
		overloads.Contains: {
			dynRuntimeName: "runtime.Contains",
			maxArguments:   2,
		},
		overloads.TimeGetFullYear: {
			dynRuntimeName: "runtime.GetFullYear",
			maxArguments:   2,
		},
		overloads.TimeGetMonth: {
			dynRuntimeName: "runtime.GetMonth",
			maxArguments:   2,
		},
		overloads.TimeGetDayOfYear: {
			dynRuntimeName: "runtime.GetDayOfYear",
			maxArguments:   2,
		},
		overloads.TimeGetDate: {
			dynRuntimeName: "runtime.GetDate",
			maxArguments:   2,
		},
		overloads.TimeGetDayOfMonth: {
			dynRuntimeName: "runtime.GetDayOfMonth",
			maxArguments:   2,
		},
		overloads.TimeGetDayOfWeek: {
			dynRuntimeName: "runtime.GetDayOfWeek",
			maxArguments:   2,
		},
		overloads.TimeGetHours: {
			dynRuntimeName: "runtime.GetHours",
			maxArguments:   2,
		},
		overloads.TimeGetMinutes: {
			dynRuntimeName: "runtime.GetMinutes",
			maxArguments:   2,
		},
		overloads.TimeGetSeconds: {
			dynRuntimeName: "runtime.GetSeconds",
			maxArguments:   2,
		},
		overloads.TimeGetMilliseconds: {
			dynRuntimeName: "runtime.GetMilliseconds",
			maxArguments:   2,
		},
		"int": {
			dynRuntimeName: "runtime.Int",
			maxArguments:   1,
		},
		"uint": {
			dynRuntimeName: "runtime.Uint",
			maxArguments:   1,
		},
		"double": {
			dynRuntimeName: "runtime.Double",
			maxArguments:   1,
		},
		"bool": {
			dynRuntimeName: "runtime.Bool",
			maxArguments:   1,
		},
		"string": {
			dynRuntimeName: "runtime.String",
			maxArguments:   1,
		},
		"bytes": {
			dynRuntimeName: "runtime.Bytes",
			maxArguments:   1,
		},
		"timestamp": {
			dynRuntimeName: "runtime.Timestamp",
			maxArguments:   1,
		},
		"duration": {
			dynRuntimeName: "runtime.Duration",
			maxArguments:   1,
		},
	}
}
