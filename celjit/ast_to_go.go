package celjit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/cel-go/common/operators"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

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

		if exprKind.CallExpr.GetTarget() != nil {
			targetGo, err := astToGoSource(exprKind.CallExpr.GetTarget())
			if err != nil {
				return "", fmt.Errorf("target: %w", err)
			}

			maybeArgGo0 := "runtime.Value{}"
			if len(argsGo) >= 1 {
				maybeArgGo0 = argsGo[0]
			}
			switch exprKind.CallExpr.GetFunction() {
			case "size":
				return fmt.Sprintf("runtime.Size(%s)", targetGo), nil
			case "contains":
				return fmt.Sprintf("runtime.Contains(%s, %s)", targetGo, argsGo[0]), nil
			case "endsWith":
				return fmt.Sprintf("runtime.EndsWith(%s, %s)", targetGo, argsGo[0]), nil
			case "matches":
				return fmt.Sprintf("runtime.Matches(%s, %s)", targetGo, argsGo[0]), nil
			case "startsWith":
				return fmt.Sprintf("runtime.StartsWith(%s, %s)", targetGo, argsGo[0]), nil
			case "getFullYear":
				return fmt.Sprintf("runtime.GetFullYear(%s, %s)", targetGo, maybeArgGo0), nil
			case "getMonth":
				return fmt.Sprintf("runtime.GetMonth(%s, %s)", targetGo, maybeArgGo0), nil
			case "getDayOfYear":
				return fmt.Sprintf("runtime.GetDayOfYear(%s, %s)", targetGo, maybeArgGo0), nil
			case "getDate":
				return fmt.Sprintf("runtime.GetDate(%s, %s)", targetGo, maybeArgGo0), nil
			case "getDayOfMonth":
				return fmt.Sprintf("runtime.GetDayOfMonth(%s, %s)", targetGo, maybeArgGo0), nil
			case "getDayOfWeek":
				return fmt.Sprintf("runtime.GetDayOfWeek(%s, %s)", targetGo, maybeArgGo0), nil
			case "getHours":
				return fmt.Sprintf("runtime.GetHours(%s, %s)", targetGo, maybeArgGo0), nil
			case "getMinutes":
				return fmt.Sprintf("runtime.GetMinutes(%s, %s)", targetGo, maybeArgGo0), nil
			case "getSeconds":
				return fmt.Sprintf("runtime.GetSeconds(%s, %s)", targetGo, maybeArgGo0), nil
			case "getMilliseconds":
				return fmt.Sprintf("runtime.GetMilliseconds(%s, %s)", targetGo, maybeArgGo0), nil
			default:
				return "", fmt.Errorf("unsupported overload %q", exprKind.CallExpr.GetFunction())
			}
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
		case "size":
			return fmt.Sprintf("runtime.Size(%s)", argsGo[0]), nil
		case "matches":
			return fmt.Sprintf("runtime.Matches(%s, %s)", argsGo[0], argsGo[1]), nil
		case "int":
			return fmt.Sprintf("runtime.Int(%s)", argsGo[0]), nil
		case "uint":
			return fmt.Sprintf("runtime.Uint(%s)", argsGo[0]), nil
		case "double":
			return fmt.Sprintf("runtime.Double(%s)", argsGo[0]), nil
		case "bool":
			return fmt.Sprintf("runtime.Bool(%s)", argsGo[0]), nil
		case "string":
			return fmt.Sprintf("runtime.String(%s)", argsGo[0]), nil
		case "bytes":
			return fmt.Sprintf("runtime.Bytes(%s)", argsGo[0]), nil
		case "timestamp":
			return fmt.Sprintf("runtime.Timestamp(%s)", argsGo[0]), nil
		case "duration":
			return fmt.Sprintf("runtime.Duration(%s)", argsGo[0]), nil
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
