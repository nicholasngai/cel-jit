package celjit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
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
			return fmt.Sprintf("runtime.IntValue{Val: int64(%d)}", constKind.Int64Value), nil
		case *expr.Constant_Uint64Value:
			return fmt.Sprintf("runtime.UintValue{Val: uint64(%d)}", constKind.Uint64Value), nil
		case *expr.Constant_DoubleValue:
			return fmt.Sprintf("runtime.DoubleValue{Val: %f}", constKind.DoubleValue), nil
		case *expr.Constant_BoolValue:
			return fmt.Sprintf("runtime.BoolValue{Val: %t}", constKind.BoolValue), nil
		case *expr.Constant_StringValue:
			return fmt.Sprintf("runtime.StringValue{Val: %q}", constKind.StringValue), nil
		case *expr.Constant_BytesValue:
			var builder strings.Builder
			builder.WriteString("runtime.BytesValue{Val: []byte{")
			for i, b := range constKind.BytesValue {
				if i > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(strconv.Itoa(int(b)))
			}
			builder.WriteString("}}")
			return builder.String(), nil
		case *expr.Constant_NullValue:
			return "(runtime.NullValue{})", nil
		// TODO(nngai) timestamp(const) and duration(const) are common enough
		// that we should probably optimize them to be constant.
		default:
			return "", fmt.Errorf("unsupported constant kind %q", exprKind.ConstExpr.GetConstantKind())
		}
	case *expr.Expr_ListExpr:
		listExprType, ok := checkedExpr.GetTypeMap()[node.GetId()]
		if !ok {
			return "", fmt.Errorf("no type info for node %d", node.GetId())
		}
		listType, err := cel.ExprTypeToType(listExprType)
		if err != nil {
			return "", fmt.Errorf("expr type %v to CEL type", listExprType)
		}
		if listType.Kind() != cel.ListKind {
			return "", fmt.Errorf("type info for node %d is %v, not list", node.GetId(), listType.Kind())
		}
		elemTypeInfo, err := celTypeInfo(listType.Parameters()[0])
		if err != nil {
			return "", fmt.Errorf("elem type %v to runtime types: %w", listType.Parameters()[0], err)
		}

		var builder strings.Builder
		fmt.Fprintf(&builder, `(func() runtime.ListValue[%s] {
			s := make([]%[1]s, %d)`,
			elemTypeInfo.goType, len(exprKind.ListExpr.GetElements()),
		)
		for i, elem := range exprKind.ListExpr.GetElements() {
			elemSource, err := astToGoSource(elem, checkedExpr)
			if err != nil {
				return "", fmt.Errorf("list elem %d: %w", i, err)
			}

			fmt.Fprintf(&builder, `
			elem%d := %s
			if elem%[1]d.Err != nil {
				return runtime.ListValue[%[3]s]{Err: elem%[1]d.Err}
			}
			s[%[1]d] = elem%[1]d.Val`,
				i, elemTypeInfo.converter(elemSource), elemTypeInfo.goType,
			)
		}
		fmt.Fprintf(&builder, `
			return runtime.ListValue[%s]{Val: s}
		})()`,
			elemTypeInfo.goType,
		)
		return builder.String(), nil
	case *expr.Expr_StructExpr:
		if exprKind.StructExpr.MessageName == "" {
			// Map.

			mapExprType, ok := checkedExpr.GetTypeMap()[node.GetId()]
			if !ok {
				return "", fmt.Errorf("no type info for node %d", node.GetId())
			}
			mapType, err := cel.ExprTypeToType(mapExprType)
			if err != nil {
				return "", fmt.Errorf("expr type %v to CEL type", mapExprType)
			}
			if mapType.Kind() != cel.MapKind {
				return "", fmt.Errorf("type info for node %d is %v, not map", node.GetId(), mapType.Kind())
			}
			keyTypeInfo, err := celTypeInfo(mapType.Parameters()[0])
			if err != nil {
				return "", fmt.Errorf("key type %v to runtime types: %w", mapType.Parameters()[0], err)
			}
			valTypeInfo, err := celTypeInfo(mapType.Parameters()[1])
			if err != nil {
				return "", fmt.Errorf("value type %v to runtime types: %w", mapType.Parameters()[1], err)
			}

			var builder strings.Builder
			fmt.Fprintf(&builder, `(func() runtime.MapValue[%s, %s] {
				s := make(map[%[1]s]%[2]s, %d)`,
				keyTypeInfo.goType, valTypeInfo.goType, len(exprKind.StructExpr.GetEntries()),
			)
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
				if key%[1]d.Err != nil {
					return runtime.MapValue[%[4]s, %[5]s]{Err: key%[1]d.Err}
				}
				val%[1]d := %[3]s
				if val%[1]d.Err != nil {
					return runtime.MapValue[%[4]s, %[5]s]{Err: val%[1]d.Err}
				}
				s[key%[1]d.Val] = val%[1]d.Val`,
					i, keyTypeInfo.converter(keySource), valTypeInfo.converter(valSource), keyTypeInfo.goType, valTypeInfo.goType,
				)
			}
			fmt.Fprintf(&builder, `
				return runtime.MapValue[%s, %s]{Val: s}
			})()`,
				keyTypeInfo.goType, valTypeInfo.goType,
			)
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
			if collection.Err != nil {
				return collection
			}

			collectionVal := reflect.ValueOf(collection.Val)
			switch collectionVal.Type().Kind() {
			case reflect.Slice:
				%[2]s := %[3]s.DynValue()
				if %[2]s.Err != nil {
					return %[2]s
				}

				for i := range collectionVal.Len() {
					%[4]s := runtime.DynValue{Val: collectionVal.Index(i).Interface()}

					cond := %[5]s.DynValue()
					if cond.Err != nil {
						return cond
					}
					if cond.Val != true {
						break
					}

					%[2]s = %[6]s.DynValue()
					if %[2]s.Err != nil {
						return %[2]s
					}
				}

				return %[7]s.DynValue()
			case reflect.Map:
				%[2]s := %[3]s.DynValue()
				if %[2]s.Err != nil {
					return %[2]s
				}

				mapIter := collectionVal.MapRange()
				for mapIter.Next() {
					%[4]s := runtime.DynValue{Val: mapIter.Key().Interface()}

					cond := %[5]s.DynValue()
					if cond.Err != nil {
						return cond
					}
					if cond.Val != true {
						break
					}

					%[2]s = %[6]s.DynValue()
					if %[2]s.Err != nil {
						return %[2]s
					}
				}

				return %[7]s.DynValue()
			default:
				return runtime.DynValue{Err: fmt.Errorf("unsupported comprehension type %%T", collectionVal)}
			}
		})()`,
			rangeGo, mangleVariable(exprKind.ComprehensionExpr.GetAccuVar()), accumulatorInitGo, mangleVariable(exprKind.ComprehensionExpr.GetIterVar()), loopCondGo, loopStepGo, resultGo,
		), nil
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
			resExprType, ok := checkedExpr.GetTypeMap()[node.GetId()]
			if !ok {
				return "", fmt.Errorf("no type info for node %d", node.GetId())
			}
			resType, err := cel.ExprTypeToType(resExprType)
			if err != nil {
				return "", fmt.Errorf("expr type %v to CEL type", resExprType)
			}
			resTypeInfo, err := celTypeInfo(resType)
			if err != nil {
				return "", fmt.Errorf("result type %v to runtime types: %w", resType, err)
			}

			return fmt.Sprintf(`(func() %s {
				cond := %s.BoolValue()
				if cond.Err != nil {
					return %[1]s{Err: cond.Err}
				}

				if cond.Val {
					return %[3]s
				} else {
					return %[4]s
				}
			})()`,
				resTypeInfo.runtimeType, argsGo[0], resTypeInfo.converter(argsGo[1]), resTypeInfo.converter(argsGo[2]),
			), nil
		case operators.LogicalAnd:
			return fmt.Sprintf(`(func() runtime.BoolValue {
				left := %s.BoolValue()
				if left.Err == nil && !left.Val {
					return left
				}
				right := %s.BoolValue()
				if right.Err == nil && !right.Val {
					return right
				}
				if left.Err != nil {
					return left
				}
				return right
			})()`,
				argsGo[0], argsGo[1],
			), nil
		case operators.LogicalOr:
			return fmt.Sprintf(`(func() runtime.BoolValue {
				left := %s.BoolValue()
				if left.Err == nil && left.Val {
					return left
				}
				right := %s.BoolValue()
				if right.Err == nil && right.Val {
					return right
				}
				if left.Err != nil {
					return left
				}
				return right
			})()`,
				argsGo[0], argsGo[1],
			), nil
		case operators.LogicalNot:
			return fmt.Sprintf("runtime.LogicalNot(%s.BoolValue())", argsGo[0]), nil
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
			case overloads.AddUint64:
				return fmt.Sprintf("runtime.AddUint64(%s.UintValue(), %s.UintValue())", argsGo[0], argsGo[1]), nil
			case overloads.AddDouble:
				return fmt.Sprintf("runtime.AddDouble(%s.DoubleValue(), %s.DoubleValue())", argsGo[0], argsGo[1]), nil
			case overloads.AddString:
				return fmt.Sprintf("runtime.AddString(%s.StringValue(), %s.StringValue())", argsGo[0], argsGo[1]), nil
			case overloads.AddBytes:
				return fmt.Sprintf("runtime.AddBytes(%s.BytesValue(), %s.BytesValue())", argsGo[0], argsGo[1]), nil
			case overloads.AddList:
				listExprType, ok := checkedExpr.GetTypeMap()[node.GetId()]
				if !ok {
					return "", fmt.Errorf("no type info for node %d", node.GetId())
				}
				listType, err := cel.ExprTypeToType(listExprType)
				if err != nil {
					return "", fmt.Errorf("expr type %v to CEL type", listExprType)
				}
				listTypeInfo, err := celTypeInfo(listType)
				if err != nil {
					return "", fmt.Errorf("elem type %v to runtime types: %w", listType, err)
				}
				return fmt.Sprintf("runtime.AddList(%s, %s)", listTypeInfo.converter(argsGo[0]), listTypeInfo.converter(argsGo[1])), nil
			case overloads.AddTimestampDuration:
				return fmt.Sprintf("runtime.AddTimestampDuration(%s.TimestampValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
			case overloads.AddDurationTimestamp:
				return fmt.Sprintf("runtime.AddTimestampDuration(%s.TimestampValue(), %s.DurationValue())", argsGo[1], argsGo[0]), nil
			case overloads.AddDurationDuration:
				return fmt.Sprintf("runtime.AddDurationDuration(%s.DurationValue(), %s.DurationValue())", argsGo[1], argsGo[0]), nil
			default:
				return fmt.Sprintf("runtime.Add(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
			}
		case operators.Subtract:
			switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
			case overloads.SubtractInt64:
				return fmt.Sprintf("runtime.SubtractInt64(%s.IntValue(), %s.IntValue())", argsGo[0], argsGo[1]), nil
			case overloads.SubtractUint64:
				return fmt.Sprintf("runtime.SubtractUint64(%s.UintValue(), %s.UintValue())", argsGo[0], argsGo[1]), nil
			case overloads.SubtractDouble:
				return fmt.Sprintf("runtime.SubtractDouble(%s.DoubleValue(), %s.DoubleValue())", argsGo[0], argsGo[1]), nil
			case overloads.SubtractTimestampTimestamp:
				return fmt.Sprintf("runtime.SubtractTimestampTimestamp(%s.TimestampValue(), %s.TimestampValue())", argsGo[0], argsGo[1]), nil
			case overloads.SubtractTimestampDuration:
				return fmt.Sprintf("runtime.SubtractTimestampDuration(%s.TimestampValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
			case overloads.SubtractDurationDuration:
				return fmt.Sprintf("runtime.SubtractDurationDuration(%s.DurationValue(), %s.DurationValue())", argsGo[1], argsGo[0]), nil
			default:
				return fmt.Sprintf("runtime.Subtract(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
			}
		case operators.Multiply:
			switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
			case overloads.MultiplyInt64:
				return fmt.Sprintf("runtime.MultiplyInt64(%s.IntValue(), %s.IntValue())", argsGo[0], argsGo[1]), nil
			case overloads.MultiplyUint64:
				return fmt.Sprintf("runtime.MultiplyUint64(%s.UintValue(), %s.UintValue())", argsGo[0], argsGo[1]), nil
			case overloads.MultiplyDouble:
				return fmt.Sprintf("runtime.MultiplyDouble(%s.DoubleValue(), %s.DoubleValue())", argsGo[0], argsGo[1]), nil
			default:
				return fmt.Sprintf("runtime.Multiply(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
			}
		case operators.Divide:
			switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
			case overloads.DivideInt64:
				return fmt.Sprintf("runtime.DivideInt64(%s.IntValue(), %s.IntValue())", argsGo[0], argsGo[1]), nil
			case overloads.DivideUint64:
				return fmt.Sprintf("runtime.DivideUint64(%s.UintValue(), %s.UintValue())", argsGo[0], argsGo[1]), nil
			case overloads.DivideDouble:
				return fmt.Sprintf("runtime.DivideDouble(%s.DoubleValue(), %s.DoubleValue())", argsGo[0], argsGo[1]), nil
			default:
				return fmt.Sprintf("runtime.Divide(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
			}
		case operators.Modulo:
			switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
			case overloads.ModuloInt64:
				return fmt.Sprintf("runtime.ModuloInt64(%s.IntValue(), %s.IntValue())", argsGo[0], argsGo[1]), nil
			case overloads.ModuloUint64:
				return fmt.Sprintf("runtime.ModuloUint64(%s.UintValue(), %s.UintValue())", argsGo[0], argsGo[1]), nil
			default:
				return fmt.Sprintf("runtime.Modulo(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
			}
		case operators.Negate:
			switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
			case overloads.DivideInt64:
				return fmt.Sprintf("runtime.NegateInt64(%s.IntValue())", argsGo[0]), nil
			case overloads.NegateDouble:
				return fmt.Sprintf("runtime.NegateDouble(%s.DoubleValue())", argsGo[0]), nil
			default:
				return fmt.Sprintf("runtime.Negate(%s.DynValue())", argsGo[0]), nil
			}
		case operators.Index:
			return fmt.Sprintf("runtime.Index(%s.DynValue(), %s.DynValue())", argsGo[0], argsGo[1]), nil
		case operators.NotStrictlyFalse:
			return fmt.Sprintf("runtime.NotStrictlyFalse(%s.BoolValue())", argsGo[0]), nil
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
			return fmt.Sprintf("%s.DynValue()", argsGo[0]), nil
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
