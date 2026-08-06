package celjit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/overloads"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

func astToGoSource(node *expr.Expr, checkedExpr *expr.CheckedExpr) (string, error) {
	switch exprKind := node.GetExprKind().(type) {
	case *expr.Expr_IdentExpr:
		return mangleVariable(exprKind.IdentExpr.GetName()), nil
	case *expr.Expr_ConstExpr:
		switch constKind := exprKind.ConstExpr.GetConstantKind().(type) {
		case *expr.Constant_Int64Value:
			return fmt.Sprintf("runtime.IntValueOf(int64(%d))", constKind.Int64Value), nil
		case *expr.Constant_Uint64Value:
			return fmt.Sprintf("runtime.UintValueOf(uint64(%d))", constKind.Uint64Value), nil
		case *expr.Constant_DoubleValue:
			return fmt.Sprintf("runtime.DoubleValueOf(%f)", constKind.DoubleValue), nil
		case *expr.Constant_BoolValue:
			return fmt.Sprintf("runtime.BoolValueOf(%t)", constKind.BoolValue), nil
		case *expr.Constant_StringValue:
			return fmt.Sprintf("runtime.StringValueOf(%q)", constKind.StringValue), nil
		case *expr.Constant_BytesValue:
			var builder strings.Builder
			builder.WriteString("runtime.BytesValueOf([]byte{")
			for i, b := range constKind.BytesValue {
				if i > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(strconv.Itoa(int(b)))
			}
			builder.WriteString("})")
			return builder.String(), nil
		case *expr.Constant_NullValue:
			return "(runtime.NullValue{})", nil
		// TODO(nngai) timestamp(const) and duration(const) are common enough
		// that we should probably optimize them to be constant.
		default:
			return "", fmt.Errorf("unsupported constant kind %q", exprKind.ConstExpr.GetConstantKind())
		}
	case *expr.Expr_ListExpr:
		var builder strings.Builder
		fmt.Fprintf(&builder, `(func() runtime.DynValue {
			s := make([]any, %d)`,
		len(exprKind.ListExpr.GetElements()))
		for i, elem := range exprKind.ListExpr.GetElements() {
			elemSource, err := astToGoSource(elem, checkedExpr)
			if err != nil {
				return "", fmt.Errorf("list elem %d: %w", i, err)
			}

			fmt.Fprintf(&builder, `
			elem%d := %s
			if elem%[1]d.Err() != nil {
				return elem%[1]d.DynValue()
			}
			s[%[1]d] = elem%[1]d.Val()`,
			i, elemSource)
		}
		builder.WriteString(`
			return runtime.DynValueOf(s)
		})()`)
		return builder.String(), nil
	case *expr.Expr_StructExpr:
		if exprKind.StructExpr.MessageName == "" {
			// Map.
			var builder strings.Builder
			fmt.Fprintf(&builder, `(func() runtime.DynValue {
				s := make(map[any]any, %d)`,
			len(exprKind.StructExpr.GetEntries()))
			for i, entry := range exprKind.StructExpr.GetEntries() {
				keySource, err := astToGoSource(entry.GetMapKey(), checkedExpr)
				if err != nil {
					return "", fmt.Errorf("map key %d: %w", i, err)
				}

				valSource, err := astToGoSource(entry.GetValue(), checkedExpr)
				if err != nil {
					return "", fmt.Errorf("map value %d: %w", i, err)
				}

				fmt.Fprintf(&builder, `
				key%d := %s
				if key%[1]d.Err() != nil {
					return key%[1]d.DynValue()
				}
				val%[1]d := %[3]s
				if val%[1]d.Err() != nil {
					return val%[1]d.DynValue()
				}
				s[key%[1]d.Val()] = val%[1]d.Val()`,
				i, keySource, valSource)
			}
			builder.WriteString(`
				return runtime.DynValueOf(s)
			})()`)
			return builder.String(), nil
		} else {
			// Message.
			return "", errors.New("message literals unsupported")
		}
	case *expr.Expr_SelectExpr:
		operandGo, err := astToGoSource(exprKind.SelectExpr.GetOperand(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("operand: %w", err)
		}
		if exprKind.SelectExpr.TestOnly {
			return fmt.Sprintf("runtime.Has(%s.DynValue(), %q)", operandGo, exprKind.SelectExpr.GetField()), nil
		}
		return fmt.Sprintf("runtime.Select(%s.DynValue(), %q)", operandGo, exprKind.SelectExpr.GetField()), nil
	case *expr.Expr_ComprehensionExpr:
		rangeGo, err := astToGoSource(exprKind.ComprehensionExpr.GetIterRange(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("range: %w", err)
		}

		accumulatorInitGo, err := astToGoSource(exprKind.ComprehensionExpr.GetAccuInit(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("accumulator init: %w", err)
		}

		loopStepGo, err := astToGoSource(exprKind.ComprehensionExpr.GetLoopStep(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("loop step: %w", err)
		}

		loopCondGo, err := astToGoSource(exprKind.ComprehensionExpr.GetLoopCondition(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("loop condition: %w", err)
		}

		resultGo, err := astToGoSource(exprKind.ComprehensionExpr.GetResult(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("result: %w", err)
		}

		return fmt.Sprintf(`(func() runtime.DynValue {
			collection := %[1]s.DynValue()
			if collection.Err() != nil {
				return collection
			}

			collectionVal := reflect.ValueOf(collection.Val())
			switch collectionVal.Type().Kind() {
			case reflect.Slice:
				%[2]s := %[3]s.DynValue()
				if %[2]s.Err() != nil {
					return %[2]s
				}

				for i := range collectionVal.Len() {
					%[4]s := runtime.DynValueOf(collectionVal.Index(i).Interface())

					cond := %[5]s.DynValue()
					if cond.Err() != nil {
						return cond
					}
					if cond.Val() != true {
						break
					}

					%[2]s = %[6]s.DynValue()
					if %[2]s.Err() != nil {
						return %[2]s
					}
				}

				return %[7]s.DynValue()
			case reflect.Map:
				%[2]s := %[3]s.DynValue()
				if %[2]s.Err() != nil {
					return %[2]s
				}

				mapIter := collectionVal.MapRange()
				for mapIter.Next() {
					%[4]s := runtime.DynValueOf(mapIter.Key().Interface())

					cond := %[5]s.DynValue()
					if cond.Err() != nil {
						return cond
					}
					if cond.Val() != true {
						break
					}

					%[2]s = %[6]s.DynValue()
					if %[2]s.Err() != nil {
						return %[2]s
					}
				}

				return %[7]s.DynValue()
			default:
				return runtime.DynErrorOf(fmt.Errorf("unsupported comprehension type %%T", collectionVal))
			}
		})()`, rangeGo, mangleVariable(exprKind.ComprehensionExpr.GetAccuVar()), accumulatorInitGo, mangleVariable(exprKind.ComprehensionExpr.GetIterVar()), loopCondGo, loopStepGo, resultGo), nil
	case *expr.Expr_CallExpr:
		// Arguments.
		argsGo := make([]string, 0, len(exprKind.CallExpr.GetArgs()))
		for i, arg := range exprKind.CallExpr.GetArgs() {
			argGo, err := astToGoSource(arg, checkedExpr)
			if err != nil {
				return "", fmt.Errorf("args[%d]: %w", i, err)
			}
			argsGo = append(argsGo, argGo)
		}

		if exprKind.CallExpr.GetTarget() != nil {
			targetGo, err := astToGoSource(exprKind.CallExpr.GetTarget(), checkedExpr)
			if err != nil {
				return "", fmt.Errorf("target: %w", err)
			}

			maybeArgGo0 := "runtime.DynValue{}"
			if len(argsGo) >= 1 {
				maybeArgGo0 = argsGo[0]
			}
			switch exprKind.CallExpr.GetFunction() {
			case "size":
				return fmt.Sprintf("runtime.Size(%s.DynValue())", targetGo), nil
			case "contains":
				return fmt.Sprintf("runtime.Contains(%s.DynValue(), %s.DynValue())", targetGo, argsGo[0]), nil
			case "endsWith":
				return fmt.Sprintf("runtime.EndsWith(%s.DynValue(), %s.DynValue())", targetGo, argsGo[0]), nil
			case "matches":
				return fmt.Sprintf("runtime.Matches(%s.DynValue(), %s.DynValue())", targetGo, argsGo[0]), nil
			case "startsWith":
				return fmt.Sprintf("runtime.StartsWith(%s.DynValue(), %s.DynValue())", targetGo, argsGo[0]), nil
			case "getFullYear":
				return fmt.Sprintf("runtime.GetFullYear(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getMonth":
				return fmt.Sprintf("runtime.GetMonth(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getDayOfYear":
				return fmt.Sprintf("runtime.GetDayOfYear(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getDate":
				return fmt.Sprintf("runtime.GetDate(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getDayOfMonth":
				return fmt.Sprintf("runtime.GetDayOfMonth(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getDayOfWeek":
				return fmt.Sprintf("runtime.GetDayOfWeek(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getHours":
				return fmt.Sprintf("runtime.GetHours(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getMinutes":
				return fmt.Sprintf("runtime.GetMinutes(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getSeconds":
				return fmt.Sprintf("runtime.GetSeconds(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			case "getMilliseconds":
				return fmt.Sprintf("runtime.GetMilliseconds(%s.DynValue(), %s.DynValue())", targetGo, maybeArgGo0), nil
			default:
				return "", fmt.Errorf("unsupported overload %q", exprKind.CallExpr.GetFunction())
			}
		}

		switch exprKind.CallExpr.GetFunction() {
		case operators.Conditional:
			return fmt.Sprintf(`(func() runtime.DynValue {
				cond := %s.DynValue()
				if cond.Err() != nil {
					return cond
				}

				if cond.Val() == true {
					return %s.DynValue()
				} else {
					return %s.DynValue()
				}
			})()`, argsGo[0], argsGo[1], argsGo[2]), nil
		case operators.LogicalAnd:
			return fmt.Sprintf("runtime.LogicalAnd(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.LogicalOr:
			return fmt.Sprintf("runtime.LogicalOr(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.LogicalNot:
			return fmt.Sprintf("runtime.LogicalNot(%s.DynValue())", argsGo[0]), nil
		case operators.Equals:
			return fmt.Sprintf("runtime.Equals(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.NotEquals:
			return fmt.Sprintf("runtime.NotEquals(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.Less:
			return fmt.Sprintf("runtime.Less(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.LessEquals:
			return fmt.Sprintf("runtime.LessEquals(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.Greater:
			return fmt.Sprintf("runtime.Greater(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.GreaterEquals:
			return fmt.Sprintf("runtime.GreaterEquals(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.Add:
			switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
			case overloads.AddInt64:
				return fmt.Sprintf("runtime.AddInt64(%s.IntValue(), %s.IntValue())", argsGo[0], argsGo[1]), nil
			default:
				return fmt.Sprintf("runtime.Add(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
			}
		case operators.Subtract:
			return fmt.Sprintf("runtime.Subtract(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.Multiply:
			return fmt.Sprintf("runtime.Multiply(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.Divide:
			return fmt.Sprintf("runtime.Divide(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.Modulo:
			return fmt.Sprintf("runtime.Modulo(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.Negate:
			return fmt.Sprintf("runtime.Negate(%s.DynValue())", argsGo[0]), nil
		case operators.Index:
			return fmt.Sprintf("runtime.Index(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.NotStrictlyFalse:
			return fmt.Sprintf("runtime.NotStrictlyFalse(%s.DynValue())", argsGo[0]), nil
		case operators.In:
			return fmt.Sprintf("runtime.In(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case "size":
			return fmt.Sprintf("runtime.Size(%s.DynValue())", argsGo[0]), nil
		case "matches":
			return fmt.Sprintf("runtime.Matches(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case "int":
			return fmt.Sprintf("runtime.Int(%s.DynValue())", argsGo[0]), nil
		case "uint":
			return fmt.Sprintf("runtime.Uint(%s.DynValue())", argsGo[0]), nil
		case "double":
			return fmt.Sprintf("runtime.Double(%s.DynValue())", argsGo[0]), nil
		case "bool":
			return fmt.Sprintf("runtime.Bool(%s.DynValue())", argsGo[0]), nil
		case "string":
			return fmt.Sprintf("runtime.String(%s.DynValue())", argsGo[0]), nil
		case "bytes":
			return fmt.Sprintf("runtime.Bytes(%s.DynValue())", argsGo[0]), nil
		case "timestamp":
			return fmt.Sprintf("runtime.Timestamp(%s.DynValue())", argsGo[0]), nil
		case "duration":
			return fmt.Sprintf("runtime.Duration(%s.DynValue())", argsGo[0]), nil
		case "dyn":
			return argsGo[0], nil
		default:
			return "", fmt.Errorf("unsupported function %q", exprKind.CallExpr.GetFunction())
		}
	default:
		return "", fmt.Errorf("unsupported expr kind %v", node)
	}
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

func extractOverloadID(overloadIDs []string) string {
	// We can only optimize to an overload variant if there is a single overload
	// identified by CEL. Else we fall back to dynamic.
	if len(overloadIDs) != 1 {
		return ""
	}
	return overloadIDs[0]
}
