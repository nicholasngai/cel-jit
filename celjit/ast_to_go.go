package celjit

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/operators"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

type astWriter struct {
	env    *Env
	valIdx int
}

func (aw *astWriter) writeGoSourceForAst(w io.Writer, node *expr.Expr, checkedExpr *expr.CheckedExpr) (string, error) {
	switch exprKind := node.GetExprKind().(type) {
	case *expr.Expr_IdentExpr:
		return mangleVariable(exprKind.IdentExpr.GetName()), nil
	case *expr.Expr_ConstExpr:
		switch constKind := exprKind.ConstExpr.GetConstantKind().(type) {
		case *expr.Constant_Int64Value:
			return fmt.Sprintf("int64(%d)", constKind.Int64Value), nil
		case *expr.Constant_Uint64Value:
			return fmt.Sprintf("uint64(%d)", constKind.Uint64Value), nil
		case *expr.Constant_DoubleValue:
			return fmt.Sprintf("%f", constKind.DoubleValue), nil
		case *expr.Constant_BoolValue:
			return fmt.Sprintf("%t", constKind.BoolValue), nil
		case *expr.Constant_StringValue:
			return fmt.Sprintf("%q", constKind.StringValue), nil
		case *expr.Constant_BytesValue:
			var builder strings.Builder
			builder.WriteString("[]byte{")
			for i, b := range constKind.BytesValue {
				if i > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(strconv.Itoa(int(b)))
			}
			builder.WriteString("}")
			return builder.String(), nil
		case *expr.Constant_NullValue:
			return "struct{}{}", nil
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
		fmt.Fprintf(&builder, "[]%s{", elemTypeInfo.goType)
		for i, elem := range exprKind.ListExpr.GetElements() {
			if i > 0 {
				builder.WriteString(", ")
			}

			elemSource, err := aw.writeGoSourceForAst(w, elem, checkedExpr)
			if err != nil {
				return "", fmt.Errorf("list elem %d: %w", i, err)
			}

			builder.WriteString(elemSource)
		}
		builder.WriteString("}")

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
			fmt.Fprintf(&builder, "map[%s]%s{", keyTypeInfo.goType, valTypeInfo.goType)
			for i, entry := range exprKind.StructExpr.GetEntries() {
				if i > 0 {
					builder.WriteString(", ")
				}

				keySource, err := aw.writeGoSourceForAst(w, entry.GetMapKey(), checkedExpr)
				if err != nil {
					return "", fmt.Errorf("map key %d: %w", i, err)
				}

				valSource, err := aw.writeGoSourceForAst(w, entry.GetValue(), checkedExpr)
				if err != nil {
					return "", fmt.Errorf("map value %d: %w", i, err)
				}

				fmt.Fprintf(&builder, "%s: %s", keySource, valSource)
			}
			builder.WriteString("}")

			return builder.String(), nil
		} else {
			// Message.
			return "", errors.New("message literals unsupported")
		}
	case *expr.Expr_SelectExpr:
		operandGo, err := aw.writeGoSourceForAst(w, exprKind.SelectExpr.GetOperand(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("operand: %w", err)
		}

		operandExprType, ok := checkedExpr.GetTypeMap()[exprKind.SelectExpr.GetOperand().GetId()]
		if !ok {
			return "", fmt.Errorf("no type info for node %d", exprKind.SelectExpr.GetOperand().GetId())
		}
		operandType, err := cel.ExprTypeToType(operandExprType)
		if err != nil {
			return "", fmt.Errorf("expr type %v to CEL type", operandExprType)
		}

		if operandType.Kind() != cel.MapKind {
			if exprKind.SelectExpr.TestOnly {
				return fmt.Sprintf("runtime.Has(%s, %q)", operandGo, exprKind.SelectExpr.GetField()), nil
			} else {
				return aw.handleErr(w, fmt.Sprintf("runtime.Select(%s, %q)", operandGo, exprKind.SelectExpr.GetField())), nil
			}
		}

		operandTypeInfo, err := celTypeInfo(operandType)
		if err != nil {
			return "", fmt.Errorf("operand type %v to runtime types: %w", operandType, err)
		}

		switch operandType.Parameters()[0] {
		case cel.StringType:
			if exprKind.SelectExpr.TestOnly {
				return fmt.Sprintf("runtime.HasMap(%s, %q)", operandGo, exprKind.SelectExpr.GetField()), nil
			} else {
				return aw.handleErr(w, fmt.Sprintf("runtime.SelectMap(%s, %q)", operandTypeInfo.converter(aw, w, operandGo), exprKind.SelectExpr.GetField())), nil
			}
		default:
			if exprKind.SelectExpr.TestOnly {
				return fmt.Sprintf("runtime.Has(%s, %q)", operandGo, exprKind.SelectExpr.GetField()), nil
			} else {
				return aw.handleErr(w, fmt.Sprintf("runtime.Select(%s, %q)", operandGo, exprKind.SelectExpr.GetField())), nil
			}
		}
	case *expr.Expr_ComprehensionExpr:
		rangeGo, err := aw.writeGoSourceForAst(w, exprKind.ComprehensionExpr.GetIterRange(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("range: %w", err)
		}

		accumulatorInitGo, err := aw.writeGoSourceForAst(w, exprKind.ComprehensionExpr.GetAccuInit(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("accumulator init: %w", err)
		}

		// Write loop step and cond since this will have to be nested inside the larger statement.
		var b strings.Builder
		loopStepGo, err := aw.writeGoSourceForAst(&b, exprKind.ComprehensionExpr.GetLoopStep(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("loop step: %w", err)
		}
		loopStepStmt := b.String()
		b.Reset()
		loopCondGo, err := aw.writeGoSourceForAst(&b, exprKind.ComprehensionExpr.GetLoopCondition(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("loop condition: %w", err)
		}
		loopCondStmt := b.String()

		// TODO(nngai) Comprehension type overloads.

		fmt.Fprintf(w, `
			v%[1]dVal := reflect.ValueOf(%s)
			v%[1]dAccu := %[3]s
			switch v%[1]dVal.Type().Kind() {
			case reflect.Slice:
				for i := range collectionVal.Len() {
					%[4]s := collectionVal.Index(i).Interface()

					%s
					if %s != true {
						break
					}

					%s
					%[3]s = %[8]s
				}
			case reflect.Map:
				mapIter := collectionVal.MapRange()
				for mapIter.Next() {
					%[4]s := mapIter.Key().Interface()

					%s
					if %s != true {
						break
					}

					%s
					%[3]s = %[8]s
				}
			default:
				return zero, fmt.Errorf("unsupported comprehension type %%T", collectionVal)
			}
			`,
			aw.valIdx, rangeGo, accumulatorInitGo, mangleVariable(exprKind.ComprehensionExpr.GetIterVar()), loopCondStmt, loopCondGo, loopStepStmt, loopStepGo,
		)

		resultGo, err := aw.writeGoSourceForAst(w, exprKind.ComprehensionExpr.GetResult(), checkedExpr)
		if err != nil {
			return "", fmt.Errorf("result: %w", err)
		}

		return resultGo, nil
	case *expr.Expr_CallExpr:
		// Handle logical AND, OR, and conditional first since those have lazy
		// eval semantics.
		switch exprKind.CallExpr.GetFunction() {
	//	case operators.Conditional:
	//		resExprType, ok := checkedExpr.GetTypeMap()[node.GetId()]
	//		if !ok {
	//			return "", fmt.Errorf("no type info for node %d", node.GetId())
	//		}
	//		resType, err := cel.ExprTypeToType(resExprType)
	//		if err != nil {
	//			return "", fmt.Errorf("expr type %v to CEL type", resExprType)
	//		}
	//		resTypeInfo, err := celTypeInfo(resType)
	//		if err != nil {
	//			return "", fmt.Errorf("result type %v to runtime types: %w", resType, err)
	//		}

	//		return fmt.Sprintf(`(func() %s {
	//			cond := %s
	//			if cond.Err != nil {
	//				return %[1]s{Err: cond.Err}
	//			}

	//			if cond.Val {
	//				return %[3]s
	//			} else {
	//				return %[4]s
	//			}
	//		})()`,
	//			resTypeInfo.runtimeType, argsGo[0], resTypeInfo.converter(argsGo[1]), resTypeInfo.converter(argsGo[2]),
	//		), nil
		case operators.LogicalAnd:
			// Write left and write to builder since this will have to be nested
			// inside the larger statement.
			var b strings.Builder
			leftGo, err := aw.writeGoSourceForAst(&b, exprKind.CallExpr.GetArgs()[0], checkedExpr)
			if err != nil {
				return "", fmt.Errorf("left: %w", err)
			}
			leftStmt := b.String()
			b.Reset()
			rightGo, err := aw.writeGoSourceForAst(&b, exprKind.CallExpr.GetArgs()[1], checkedExpr)
			if err != nil {
				return "", fmt.Errorf("right: %w", err)
			}
			rightStmt := b.String()

			freindentf(w, `
				var v%[1]d bool
				v%[1]dLeft, v%[1]dLeftErr := func() (bool, error) {
					var zero bool
					_ = zero

					%s
					return any(%s).(bool), nil
				}()
				if v%[1]dLeftErr == nil && !v%[1]dLeft {
					v%[1]d = false
				} else {
					v%[1]dRight, v%[1]dRightErr := func() (bool, error) {
						var zero bool
						_ = zero

						%[4]s
						return any(%s).(bool), nil
					}()
					if v%[1]dRightErr == nil && !v%[1]dRight {
						v%[1]d = false
					} else if v%[1]dLeftErr != nil {
						return zero, v%[1]dLeftErr
					} else if v%[1]dRightErr != nil {
						return zero, v%[1]dRightErr
					} else {
						v%[1]d = true
					}
				}
				`,
				aw.valIdx, leftStmt, leftGo, rightStmt, rightGo,
			)
			ret := fmt.Sprintf("v%d", aw.valIdx)
			aw.valIdx += 1
			return ret, nil
		case operators.LogicalOr:
			// Write left and write to builder since this will have to be nested
			// inside the larger statement.
			var b strings.Builder
			leftGo, err := aw.writeGoSourceForAst(&b, exprKind.CallExpr.GetArgs()[0], checkedExpr)
			if err != nil {
				return "", fmt.Errorf("left: %w", err)
			}
			leftStmt := b.String()
			b.Reset()
			rightGo, err := aw.writeGoSourceForAst(&b, exprKind.CallExpr.GetArgs()[1], checkedExpr)
			if err != nil {
				return "", fmt.Errorf("right: %w", err)
			}
			rightStmt := b.String()

			freindentf(w, `
				var v%[1]d bool
				v%[1]dLeft, v%[1]dLeftErr := func() (bool, error) {
					var zero bool
					_ = zero

					%s
					return any(%s).(bool), nil
				}()
				if v%[1]dLeftErr == nil && v%[1]dLeft {
					v%[1]d = true
				} else {
					v%[1]dRight, v%[1]dRightErr := func() (bool, error) {
						var zero bool
						_ = zero

						%[4]s
						return any(%s).(bool), nil
					}()
					if v%[1]dRightErr == nil && v%[1]dRight {
						v%[1]d = true
					} else if v%[1]dLeftErr != nil {
						return zero, v%[1]dLeftErr
					} else if v%[1]dRightErr != nil {
						return zero, v%[1]dRightErr
					} else {
						v%[1]d = false
					}
				}
				`,
				aw.valIdx, leftStmt, leftGo, rightStmt, rightGo,
			)
			ret := fmt.Sprintf("v%d", aw.valIdx)
			aw.valIdx += 1
			return ret, nil
	//	case operators.LogicalNot:
	//		return fmt.Sprintf("runtime.LogicalNot(%s)", argsGo[0]), nil
	//	case operators.Equals, operators.NotEquals:
	//		// If the types aren't the same, fall back to dynamic type checking.
	//		leftExprType, ok := checkedExpr.GetTypeMap()[exprKind.CallExpr.GetArgs()[0].GetId()]
	//		if !ok {
	//			return "", fmt.Errorf("no type info for node %d", node.GetId())
	//		}
	//		leftType, err := cel.ExprTypeToType(leftExprType)
	//		if err != nil {
	//			return "", fmt.Errorf("expr type %v to CEL type", leftExprType)
	//		}
	//		leftTypeInfo, err := celTypeInfo(leftType)
	//		if err != nil {
	//			return "", fmt.Errorf("left type %v to runtime types: %w", leftType, err)
	//		}
	//		rightExprType, ok := checkedExpr.GetTypeMap()[exprKind.CallExpr.GetArgs()[1].GetId()]
	//		if !ok {
	//			return "", fmt.Errorf("no type info for node %d", node.GetId())
	//		}
	//		rightType, err := cel.ExprTypeToType(rightExprType)
	//		if err != nil {
	//			return "", fmt.Errorf("expr type %v to CEL type", rightExprType)
	//		}
	//		if !rightType.IsExactType(leftType) {
	//			if exprKind.CallExpr.GetFunction() == operators.Equals {
	//				return fmt.Sprintf("runtime.Equals(%s, %s)", argsGo[0], argsGo[1]), nil
	//			} else {
	//				return fmt.Sprintf("runtime.NotEquals(%s, %s)", argsGo[0], argsGo[1]), nil
	//			}
	//		}

	//		negaterIfNotEquals := ""
	//		if exprKind.CallExpr.GetFunction() == operators.NotEquals {
	//			negaterIfNotEquals = "!"
	//		}
	//		return fmt.Sprintf(`(func() runtime.BoolValue {
	//			left := %s
	//			right := %s
	//			if left.Err != nil {
	//				return runtime.BoolValue{Err: left.Err}
	//			}
	//			if right.Err != nil {
	//				return runtime.BoolValue{Err: right.Err}
	//			}
	//			return runtime.BoolValue{Val: %s%s}
	//		})()`,
	//			argsGo[0],
	//			argsGo[1],
	//			negaterIfNotEquals,
	//			leftTypeInfo.equaler("left.Val", "right.Val"),
	//		), nil
		}

		// Arguments.
		argsGo := make([]string, 0, len(exprKind.CallExpr.GetArgs())+1)
		if exprKind.CallExpr.GetTarget() != nil {
			targetGo, err := aw.writeGoSourceForAst(w, exprKind.CallExpr.GetTarget(), checkedExpr)
			if err != nil {
				return "", fmt.Errorf("target: %w", err)
			}
			argsGo = append(argsGo, targetGo)
		}
		for i, arg := range exprKind.CallExpr.GetArgs() {
			argGo, err := aw.writeGoSourceForAst(w, arg, checkedExpr)
			if err != nil {
				return "", fmt.Errorf("args[%d]: %w", i, err)
			}
			argsGo = append(argsGo, argGo)
		}

		// Handle operators. Named function calls will be handled separately below.
		switch exprKind.CallExpr.GetFunction() {
	//	case operators.Less:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.LessInt64:
	//			return fmt.Sprintf("runtime.LessInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessInt64Uint64:
	//			return fmt.Sprintf("runtime.LessInt64Uint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessInt64Double:
	//			return fmt.Sprintf("runtime.LessInt64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessUint64:
	//			return fmt.Sprintf("runtime.LessUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessUint64Int64:
	//			return fmt.Sprintf("runtime.LessUint64Int64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessUint64Double:
	//			return fmt.Sprintf("runtime.LessUint64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessDouble:
	//			return fmt.Sprintf("runtime.LessDouble(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessDoubleInt64:
	//			return fmt.Sprintf("runtime.LessDoubleInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessDoubleUint64:
	//			return fmt.Sprintf("runtime.LessDoubleUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessBool:
	//			return fmt.Sprintf("runtime.LessBool(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessString:
	//			return fmt.Sprintf("runtime.LessString(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessBytes:
	//			return fmt.Sprintf("runtime.LessBytes(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessTimestamp:
	//			return fmt.Sprintf("runtime.LessTimestamp(%s.TimestampValue(), %s.TimestampValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.LessDuration:
	//			return fmt.Sprintf("runtime.LessDuration(%s.DurationValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsInt64:
	//			return fmt.Sprintf("runtime.LessEqualsInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsInt64Uint64:
	//			return fmt.Sprintf("runtime.LessEqualsInt64Uint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		default:
	//			return fmt.Sprintf("runtime.Less(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.LessEquals:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.LessEqualsInt64Double:
	//			return fmt.Sprintf("runtime.LessEqualsInt64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsUint64:
	//			return fmt.Sprintf("runtime.LessEqualsUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsUint64Int64:
	//			return fmt.Sprintf("runtime.LessEqualsUint64Int64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsUint64Double:
	//			return fmt.Sprintf("runtime.LessEqualsUint64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsDouble:
	//			return fmt.Sprintf("runtime.LessEqualsDouble(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsDoubleInt64:
	//			return fmt.Sprintf("runtime.LessEqualsDoubleInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsDoubleUint64:
	//			return fmt.Sprintf("runtime.LessEqualsDoubleUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsBool:
	//			return fmt.Sprintf("runtime.LessEqualsBool(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsString:
	//			return fmt.Sprintf("runtime.LessEqualsString(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsBytes:
	//			return fmt.Sprintf("runtime.LessEqualsBytes(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsTimestamp:
	//			return fmt.Sprintf("runtime.LessEqualsTimestamp(%s.TimestampValue(), %s.TimestampValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.LessEqualsDuration:
	//			return fmt.Sprintf("runtime.LessEqualsDuration(%s.DurationValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
	//		default:
	//			return fmt.Sprintf("runtime.LessEquals(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.Greater:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.GreaterInt64:
	//			return fmt.Sprintf("runtime.GreaterInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterInt64Uint64:
	//			return fmt.Sprintf("runtime.GreaterInt64Uint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterInt64Double:
	//			return fmt.Sprintf("runtime.GreaterInt64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterUint64:
	//			return fmt.Sprintf("runtime.GreaterUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterUint64Int64:
	//			return fmt.Sprintf("runtime.GreaterUint64Int64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterUint64Double:
	//			return fmt.Sprintf("runtime.GreaterUint64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterDouble:
	//			return fmt.Sprintf("runtime.GreaterDouble(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterDoubleInt64:
	//			return fmt.Sprintf("runtime.GreaterDoubleInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterDoubleUint64:
	//			return fmt.Sprintf("runtime.GreaterDoubleUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterBool:
	//			return fmt.Sprintf("runtime.GreaterBool(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterString:
	//			return fmt.Sprintf("runtime.GreaterString(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterBytes:
	//			return fmt.Sprintf("runtime.GreaterBytes(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterTimestamp:
	//			return fmt.Sprintf("runtime.GreaterTimestamp(%s.TimestampValue(), %s.TimestampValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterDuration:
	//			return fmt.Sprintf("runtime.GreaterDuration(%s.DurationValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
	//		default:
	//			return fmt.Sprintf("runtime.Greater(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.GreaterEquals:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.GreaterEqualsInt64:
	//			return fmt.Sprintf("runtime.GreaterEqualsInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsInt64Uint64:
	//			return fmt.Sprintf("runtime.GreaterEqualsInt64Uint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsInt64Double:
	//			return fmt.Sprintf("runtime.GreaterEqualsInt64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsUint64:
	//			return fmt.Sprintf("runtime.GreaterEqualsUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsUint64Int64:
	//			return fmt.Sprintf("runtime.GreaterEqualsUint64Int64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsUint64Double:
	//			return fmt.Sprintf("runtime.GreaterEqualsUint64Double(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsDouble:
	//			return fmt.Sprintf("runtime.GreaterEqualsDouble(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsDoubleInt64:
	//			return fmt.Sprintf("runtime.GreaterEqualsDoubleInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsDoubleUint64:
	//			return fmt.Sprintf("runtime.GreaterEqualsDoubleUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsBool:
	//			return fmt.Sprintf("runtime.GreaterEqualsBool(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsString:
	//			return fmt.Sprintf("runtime.GreaterEqualsString(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsBytes:
	//			return fmt.Sprintf("runtime.GreaterEqualsBytes(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsTimestamp:
	//			return fmt.Sprintf("runtime.GreaterEqualsTimestamp(%s.TimestampValue(), %s.TimestampValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.GreaterEqualsDuration:
	//			return fmt.Sprintf("runtime.GreaterEqualsDuration(%s.DurationValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
	//		default:
	//			return fmt.Sprintf("runtime.GreaterEquals(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.Add:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.AddInt64:
	//			return fmt.Sprintf("runtime.AddInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.AddUint64:
	//			return fmt.Sprintf("runtime.AddUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.AddDouble:
	//			return fmt.Sprintf("runtime.AddDouble(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.AddString:
	//			return fmt.Sprintf("runtime.AddString(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.AddBytes:
	//			return fmt.Sprintf("runtime.AddBytes(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.AddList:
	//			listExprType, ok := checkedExpr.GetTypeMap()[node.GetId()]
	//			if !ok {
	//				return "", fmt.Errorf("no type info for node %d", node.GetId())
	//			}
	//			listType, err := cel.ExprTypeToType(listExprType)
	//			if err != nil {
	//				return "", fmt.Errorf("expr type %v to CEL type", listExprType)
	//			}
	//			listTypeInfo, err := celTypeInfo(listType)
	//			if err != nil {
	//				return "", fmt.Errorf("elem type %v to runtime types: %w", listType, err)
	//			}
	//			return fmt.Sprintf("runtime.AddList(%s, %s)", listTypeInfo.converter(argsGo[0]), listTypeInfo.converter(argsGo[1])), nil
	//		case overloads.AddTimestampDuration:
	//			return fmt.Sprintf("runtime.AddTimestampDuration(%s.TimestampValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.AddDurationTimestamp:
	//			return fmt.Sprintf("runtime.AddTimestampDuration(%s.TimestampValue(), %s.DurationValue())", argsGo[1], argsGo[0]), nil
	//		case overloads.AddDurationDuration:
	//			return fmt.Sprintf("runtime.AddDurationDuration(%s.DurationValue(), %s.DurationValue())", argsGo[1], argsGo[0]), nil
	//		default:
	//			return fmt.Sprintf("runtime.Add(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.Subtract:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.SubtractInt64:
	//			return fmt.Sprintf("runtime.SubtractInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.SubtractUint64:
	//			return fmt.Sprintf("runtime.SubtractUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.SubtractDouble:
	//			return fmt.Sprintf("runtime.SubtractDouble(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.SubtractTimestampTimestamp:
	//			return fmt.Sprintf("runtime.SubtractTimestampTimestamp(%s.TimestampValue(), %s.TimestampValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.SubtractTimestampDuration:
	//			return fmt.Sprintf("runtime.SubtractTimestampDuration(%s.TimestampValue(), %s.DurationValue())", argsGo[0], argsGo[1]), nil
	//		case overloads.SubtractDurationDuration:
	//			return fmt.Sprintf("runtime.SubtractDurationDuration(%s.DurationValue(), %s.DurationValue())", argsGo[1], argsGo[0]), nil
	//		default:
	//			return fmt.Sprintf("runtime.Subtract(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.Multiply:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.MultiplyInt64:
	//			return fmt.Sprintf("runtime.MultiplyInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.MultiplyUint64:
	//			return fmt.Sprintf("runtime.MultiplyUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.MultiplyDouble:
	//			return fmt.Sprintf("runtime.MultiplyDouble(%s, %s)", argsGo[0], argsGo[1]), nil
	//		default:
	//			return fmt.Sprintf("runtime.Multiply(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
		case operators.Divide:
			switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.DivideInt64:
	//			return fmt.Sprintf("runtime.DivideInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.DivideUint64:
	//			return fmt.Sprintf("runtime.DivideUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.DivideDouble:
	//			return fmt.Sprintf("runtime.DivideDouble(%s, %s)", argsGo[0], argsGo[1]), nil
			default:
				return aw.handleErr(w, fmt.Sprintf("runtime.Divide(%s, %s)", argsGo[0], argsGo[1])), nil
			}
	//	case operators.Modulo:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.ModuloInt64:
	//			return fmt.Sprintf("runtime.ModuloInt64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		case overloads.ModuloUint64:
	//			return fmt.Sprintf("runtime.ModuloUint64(%s, %s)", argsGo[0], argsGo[1]), nil
	//		default:
	//			return fmt.Sprintf("runtime.Modulo(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.Negate:
	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.DivideInt64:
	//			return fmt.Sprintf("runtime.NegateInt64(%s)", argsGo[0]), nil
	//		case overloads.NegateDouble:
	//			return fmt.Sprintf("runtime.NegateDouble(%s)", argsGo[0]), nil
	//		default:
	//			return fmt.Sprintf("runtime.Negate(%s)", argsGo[0]), nil
	//		}
	//	case operators.Index:
	//		containerExprType, ok := checkedExpr.GetTypeMap()[exprKind.CallExpr.GetArgs()[0].GetId()]
	//		if !ok {
	//			return "", fmt.Errorf("no type info for node %d", node.GetId())
	//		}
	//		containerType, err := cel.ExprTypeToType(containerExprType)
	//		if err != nil {
	//			return "", fmt.Errorf("expr type %v to CEL type", containerExprType)
	//		}
	//		containerTypeInfo, err := celTypeInfo(containerType)
	//		if err != nil {
	//			return "", fmt.Errorf("container type %v to runtime types: %w", containerType.Parameters()[0], err)
	//		}

	//		indexExprType, ok := checkedExpr.GetTypeMap()[exprKind.CallExpr.GetArgs()[1].GetId()]
	//		if !ok {
	//			return "", fmt.Errorf("no type info for node %d", node.GetId())
	//		}
	//		indexType, err := cel.ExprTypeToType(indexExprType)
	//		if err != nil {
	//			return "", fmt.Errorf("expr type %v to CEL type", indexExprType)
	//		}

	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.IndexList:
	//			switch indexType {
	//			case cel.IntType:
	//				if containerType.Kind() != cel.ListKind {
	//					return fmt.Sprintf("runtime.Index(%s, %s)", argsGo[0], argsGo[1]), nil
	//				}
	//				elemTypeInfo, err := celTypeInfo(containerType.Parameters()[0])
	//				if err != nil {
	//					return "", fmt.Errorf("list elem type %v to runtime types: %w", containerType.Parameters()[0], err)
	//				}
	//				return fmt.Sprintf(`runtime.IndexList[%s](%s, %s)`, elemTypeInfo.runtimeType, containerTypeInfo.converter(argsGo[0]), argsGo[1]), nil
	//			default:
	//				return fmt.Sprintf("runtime.Index(%s, %s)", argsGo[0], argsGo[1]), nil
	//			}
	//		case overloads.IndexMap:
	//			switch indexType {
	//			case cel.IntType, cel.UintType, cel.StringType:
	//				if containerType.Kind() != cel.MapKind {
	//					return fmt.Sprintf("runtime.Index(%s, %s)", argsGo[0], argsGo[1]), nil
	//				}
	//				keyTypeInfo, err := celTypeInfo(containerType.Parameters()[0])
	//				if err != nil {
	//					return "", fmt.Errorf("map key type %v to runtime types: %w", containerType.Parameters()[0], err)
	//				}
	//				valTypeInfo, err := celTypeInfo(containerType.Parameters()[1])
	//				if err != nil {
	//					return "", fmt.Errorf("map value type %v to runtime types: %w", containerType.Parameters()[1], err)
	//				}
	//				return fmt.Sprintf(`runtime.IndexMap[%s](%s, %s)`, valTypeInfo.runtimeType, containerTypeInfo.converter(argsGo[0]), keyTypeInfo.converter(argsGo[1])), nil
	//			default:
	//				return fmt.Sprintf("runtime.Index(%s, %s)", argsGo[0], argsGo[1]), nil
	//			}
	//		default:
	//			return fmt.Sprintf("runtime.Index(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case operators.NotStrictlyFalse:
	//		return fmt.Sprintf("runtime.NotStrictlyFalse(%s)", argsGo[0]), nil
	//	case operators.In:
	//		searchExprType, ok := checkedExpr.GetTypeMap()[exprKind.CallExpr.GetArgs()[0].GetId()]
	//		if !ok {
	//			return "", fmt.Errorf("no type info for node %d", node.GetId())
	//		}
	//		searchType, err := cel.ExprTypeToType(searchExprType)
	//		if err != nil {
	//			return "", fmt.Errorf("expr type %v to CEL type", searchExprType)
	//		}

	//		containerExprType, ok := checkedExpr.GetTypeMap()[exprKind.CallExpr.GetArgs()[1].GetId()]
	//		if !ok {
	//			return "", fmt.Errorf("no type info for node %d", node.GetId())
	//		}
	//		containerType, err := cel.ExprTypeToType(containerExprType)
	//		if err != nil {
	//			return "", fmt.Errorf("expr type %v to CEL type", containerExprType)
	//		}
	//		containerTypeInfo, err := celTypeInfo(containerType)
	//		if err != nil {
	//			return "", fmt.Errorf("container type %v to runtime types: %w", searchType.Parameters()[0], err)
	//		}

	//		switch extractOverloadID(checkedExpr.GetReferenceMap()[node.GetId()].GetOverloadId()) {
	//		case overloads.InList:
	//			if containerType.Kind() != cel.ListKind {
	//				return fmt.Sprintf("runtime.In(%s, %s)", argsGo[0], argsGo[1]), nil
	//			}
	//			if !containerType.Parameters()[0].IsExactType(searchType) {
	//				return fmt.Sprintf("runtime.InList(%s, %s)", argsGo[0], argsGo[1]), nil
	//			}
	//			searchTypeInfo, err := celTypeInfo(searchType)
	//			if err != nil {
	//				return "", fmt.Errorf("search type %v to runtime types: %w", searchType, err)
	//			}

	//			return fmt.Sprintf(`(func() runtime.BoolValue {
	//				left := %s
	//				right := %s
	//				if left.Err != nil {
	//					return runtime.BoolValue{Err: left.Err}
	//				}
	//				if right.Err != nil {
	//					return runtime.BoolValue{Err: right.Err}
	//				}
	//				for _, elem := range right.Val {
	//					if %s {
	//						return runtime.BoolValue{Val: true}
	//					}
	//				}
	//				return runtime.BoolValue{Val: false}
	//			})()`,
	//				argsGo[0],
	//				argsGo[1],
	//				searchTypeInfo.equaler("elem", "left.Val"),
	//			), nil
	//		case overloads.InMap:
	//			switch searchType {
	//			case cel.IntType, cel.UintType, cel.StringType:
	//				if containerType.Kind() != cel.MapKind {
	//					return fmt.Sprintf("runtime.In(%s, %s)", argsGo[0], argsGo[1]), nil
	//				}
	//				keyTypeInfo, err := celTypeInfo(containerType.Parameters()[0])
	//				if err != nil {
	//					return "", fmt.Errorf("map key type %v to runtime types: %w", containerType.Parameters()[0], err)
	//				}
	//				valTypeInfo, err := celTypeInfo(containerType.Parameters()[1])
	//				if err != nil {
	//					return "", fmt.Errorf("map value type %v to runtime types: %w", containerType.Parameters()[1], err)
	//				}

	//				return fmt.Sprintf(`runtime.InMap[%s, %s, %s](%s, %s)`, keyTypeInfo.goType, valTypeInfo.goType, keyTypeInfo.runtimeType, keyTypeInfo.converter(argsGo[0]), containerTypeInfo.converter(argsGo[1])), nil
	//			default:
	//				return fmt.Sprintf("runtime.In(%s, %s)", argsGo[0], argsGo[1]), nil
	//			}
	//		default:
	//			return fmt.Sprintf("runtime.In(%s, %s)", argsGo[0], argsGo[1]), nil
	//		}
	//	case "size":
	//		return fmt.Sprintf("runtime.Size(%s)", argsGo[0]), nil
	//	case "matches":
	//		return fmt.Sprintf("runtime.Matches(%s, %s)", argsGo[0], argsGo[1]), nil
	//	case "int":
	//		return fmt.Sprintf("runtime.Int(%s)", argsGo[0]), nil
	//	case "uint":
	//		return fmt.Sprintf("runtime.Uint(%s)", argsGo[0]), nil
	//	case "double":
	//		return fmt.Sprintf("runtime.Double(%s)", argsGo[0]), nil
	//	case "bool":
	//		return fmt.Sprintf("runtime.Bool(%s)", argsGo[0]), nil
	//	case "string":
	//		return fmt.Sprintf("runtime.String(%s)", argsGo[0]), nil
	//	case "bytes":
	//		return fmt.Sprintf("runtime.Bytes(%s)", argsGo[0]), nil
	//	case "timestamp":
	//		return fmt.Sprintf("runtime.Timestamp(%s)", argsGo[0]), nil
	//	case "duration":
	//		return fmt.Sprintf("runtime.Duration(%s)", argsGo[0]), nil
		case "dyn":
			return argsGo[0], nil
		default:
			return "", fmt.Errorf("unsupported function %q", exprKind.CallExpr.GetFunction())
		}

	//	// This is a named function.
	//	funcConfig, ok := e.functions[exprKind.CallExpr.GetFunction()]
	//	if !ok {
	//		return "", fmt.Errorf("unsupported function %q", exprKind.CallExpr.GetFunction())
	//	}
	//	if len(argsGo) > funcConfig.maxArguments {
	//		return "", fmt.Errorf("function call %q has %d args, max configured is %d", exprKind.CallExpr.GetFunction(), len(argsGo), funcConfig.maxArguments)
	//	}

	//	// TODO(nngai) Check overloads for named functions.

	//	// Use the dynamic function name.
	//	var b strings.Builder
	//	b.WriteString(funcConfig.dynRuntimeName)
	//	b.WriteString("(")
	//	for i := range funcConfig.maxArguments {
	//		if i > 0 {
	//			b.WriteString(", ")
	//		}
	//		if i < len(argsGo) {
	//			b.WriteString(argsGo[i])
	//			b.WriteString("")
	//		} else {
	//			b.WriteString("runtime.DynValue{}")
	//		}
	//	}
	//	b.WriteString(")")
	//	return b.String(), nil
	default:
		return "", fmt.Errorf("unsupported expr kind %v", node)
	}
}

func (aw *astWriter) handleErr(w io.Writer, goSource string) string {
	freindentf(w, `
		v%d, err := %s
		if err != nil {
			return zero, err
		}
		`,
		aw.valIdx, goSource,
	)
	ret := fmt.Sprintf("v%d", aw.valIdx)
	aw.valIdx += 1
	return ret
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
