package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"math"
)

func SelectMap[Val ~struct{Val V; Err error}, V any](a MapValue[string, V], fieldName string) Val {
	if a.Err != nil {
		return Val{Err: a.Err}
	}

	elem, ok := a.Val[fieldName]
	if !ok {
		return Val{Err: fmt.Errorf("no such key %v", fieldName)}
	}
	return Val{Val: elem}
}

func HasMap[V any](a MapValue[string, V], fieldName string) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}

	_, ok := a.Val[fieldName]
	return BoolValue{Val: ok}
}

func LessInt64(a, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < b.Val}
}

func LessInt64Uint64(a IntValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < 0 || uint64(a.Val) < b.Val}
}

func LessInt64Double(a IntValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) < b.Val}
}

func LessUint64(a, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < b.Val}
}

func LessUint64Int64(a UintValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: b.Val >= 0 && a.Val < uint64(b.Val)}
}

func LessUint64Double(a UintValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) < b.Val}
}

func LessDouble(a, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < b.Val}
}

func LessDoubleInt64(a DoubleValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < float64(b.Val)}
}

func LessDoubleUint64(a DoubleValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < float64(b.Val)}
}

func LessBool(a, b BoolValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: !a.Val && b.Val}
}

func LessString(a, b StringValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < b.Val}
}

func LessBytes(a, b BytesValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: bytes.Compare(a.Val, b.Val) < 0}
}

func LessTimestamp(a, b TimestampValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val.Compare(b.Val) < 0}
}

func LessDuration(a, b DurationValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < b.Val}
}

func LessEqualsInt64(a, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val <= b.Val}
}

func LessEqualsInt64Uint64(a IntValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val < 0 || uint64(a.Val) <= b.Val}
}

func LessEqualsInt64Double(a IntValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) <= b.Val}
}

func LessEqualsUint64(a, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val <= b.Val}
}

func LessEqualsUint64Int64(a UintValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: b.Val >= 0 && a.Val <= uint64(b.Val)}
}

func LessEqualsUint64Double(a UintValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) <= b.Val}
}

func LessEqualsDouble(a, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val <= b.Val}
}

func LessEqualsDoubleInt64(a DoubleValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val <= float64(b.Val)}
}

func LessEqualsDoubleUint64(a DoubleValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val <= float64(b.Val)}
}

func LessEqualsBool(a, b BoolValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: !a.Val || b.Val}
}

func LessEqualsString(a, b StringValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val <= b.Val}
}

func LessEqualsBytes(a, b BytesValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: bytes.Compare(a.Val, b.Val) <= 0}
}

func LessEqualsTimestamp(a, b TimestampValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val.Compare(b.Val) <= 0}
}

func LessEqualsDuration(a, b DurationValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val <= b.Val}
}

func GreaterInt64(a, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val > b.Val}
}

func GreaterInt64Uint64(a IntValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= 0 && uint64(a.Val) > b.Val}
}

func GreaterInt64Double(a IntValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) > b.Val}
}

func GreaterUint64(a, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val > b.Val}
}

func GreaterUint64Int64(a UintValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: b.Val < 0 || a.Val > uint64(b.Val)}
}

func GreaterUint64Double(a UintValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) > b.Val}
}

func GreaterDouble(a, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val > b.Val}
}

func GreaterDoubleInt64(a DoubleValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val > float64(b.Val)}
}

func GreaterDoubleUint64(a DoubleValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val > float64(b.Val)}
}

func GreaterBool(a, b BoolValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val && !b.Val}
}

func GreaterString(a, b StringValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val > b.Val}
}

func GreaterBytes(a, b BytesValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: bytes.Compare(a.Val, b.Val) > 0}
}

func GreaterTimestamp(a, b TimestampValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val.Compare(b.Val) > 0}
}

func GreaterDuration(a, b DurationValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val > b.Val}
}

func GreaterEqualsInt64(a, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= b.Val}
}

func GreaterEqualsInt64Uint64(a IntValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= 0 && uint64(a.Val) >= b.Val}
}

func GreaterEqualsInt64Double(a IntValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) >= b.Val}
}

func GreaterEqualsUint64(a, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= b.Val}
}

func GreaterEqualsUint64Int64(a UintValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: b.Val < 0 || a.Val >= uint64(b.Val)}
}

func GreaterEqualsUint64Double(a UintValue, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: float64(a.Val) >= b.Val}
}

func GreaterEqualsDouble(a, b DoubleValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= b.Val}
}

func GreaterEqualsDoubleInt64(a DoubleValue, b IntValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= float64(b.Val)}
}

func GreaterEqualsDoubleUint64(a DoubleValue, b UintValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= float64(b.Val)}
}

func GreaterEqualsBool(a, b BoolValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val || !b.Val}
}

func GreaterEqualsString(a, b StringValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= b.Val}
}

func GreaterEqualsBytes(a, b BytesValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: bytes.Compare(a.Val, b.Val) >= 0}
}

func GreaterEqualsTimestamp(a, b TimestampValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val.Compare(b.Val) >= 0}
}

func GreaterEqualsDuration(a, b DurationValue) BoolValue {
	if a.Err != nil {
		return BoolValue{Err: a.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	return BoolValue{Val: a.Val >= b.Val}
}

func AddInt64(a, b IntValue) IntValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return IntValue{Val: a.Val + b.Val}
}

func AddUint64(a, b UintValue) UintValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return UintValue{Val: a.Val + b.Val}
}

func AddDouble(a, b DoubleValue) DoubleValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return DoubleValue{Val: a.Val + b.Val}
}

func AddString(a, b StringValue) StringValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return StringValue{Val: a.Val + b.Val}
}

func AddBytes(a, b BytesValue) BytesValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return BytesValue{Val: append(a.Val, b.Val...)}
}

func AddList[T any](a, b ListValue[T]) ListValue[T] {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return ListValue[T]{Val: append(a.Val, b.Val...)}
}

func AddTimestampDuration(a TimestampValue, b DurationValue) TimestampValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return TimestampValue{Err: b.Err}
	}
	return TimestampValue{Val: a.Val.Add(b.Val)}
}

func AddDurationDuration(a DurationValue, b DurationValue) DurationValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return DurationValue{Val: a.Val + b.Val}
}

func SubtractInt64(a, b IntValue) IntValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return IntValue{Val: a.Val - b.Val}
}

func SubtractUint64(a, b UintValue) UintValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return UintValue{Val: a.Val - b.Val}
}

func SubtractDouble(a, b DoubleValue) DoubleValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return DoubleValue{Val: a.Val - b.Val}
}

func SubtractTimestampTimestamp(a TimestampValue, b TimestampValue) DurationValue {
	if a.Err != nil {
		return DurationValue{Err: a.Err}
	}
	if b.Err != nil {
		return DurationValue{Err: b.Err}
	}
	return DurationValue{Val: a.Val.Sub(b.Val)}
}

func SubtractTimestampDuration(a TimestampValue, b DurationValue) TimestampValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return TimestampValue{Err: b.Err}
	}
	return TimestampValue{Val: a.Val.Add(-b.Val)}
}

func SubtractDurationDuration(a DurationValue, b DurationValue) DurationValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return DurationValue{Val: a.Val - b.Val}
}

func MultiplyInt64(a, b IntValue) IntValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return IntValue{Val: a.Val * b.Val}
}

func MultiplyUint64(a, b UintValue) UintValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return UintValue{Val: a.Val * b.Val}
}

func MultiplyDouble(a, b DoubleValue) DoubleValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return DoubleValue{Val: a.Val * b.Val}
}

func DivideInt64(a, b IntValue) IntValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	if b.Val == 0 {
		return IntValue{Err: errors.New("divide by 0")}
	}
	return IntValue{Val: a.Val / b.Val}
}

func DivideUint64(a, b UintValue) UintValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	if b.Val == 0 {
		return UintValue{Err: errors.New("divide by 0")}
	}
	return UintValue{Val: a.Val / b.Val}
}

func DivideDouble(a, b DoubleValue) DoubleValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	return DoubleValue{Val: a.Val / b.Val}
}

func ModuloInt64(a, b IntValue) IntValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	if b.Val == 0 {
		return IntValue{Err: errors.New("divide by 0")}
	}
	return IntValue{Val: a.Val % b.Val}
}

func ModuloUint64(a, b UintValue) UintValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}
	if b.Val == 0 {
		return UintValue{Err: errors.New("divide by 0")}
	}
	return UintValue{Val: a.Val % b.Val}
}

func NegateInt64(a IntValue) IntValue {
	if a.Err != nil {
		return a
	}
	return IntValue{Val: -a.Val}
}

func NegateDouble(a DoubleValue) DoubleValue {
	if a.Err != nil {
		return a
	}
	return DoubleValue{Val: -a.Val}
}

func IndexList[Val ~struct{Val T; Err error}, T any](a ListValue[T], b IntValue) Val {
	if a.Err != nil {
		return Val{Err: a.Err}
	}
	if b.Err != nil {
		return Val{Err: b.Err}
	}

	// Check bounds.
	if b.Val < 0 || b.Val >= int64(len(a.Val)) {
		return Val{Err: fmt.Errorf("index %d out of range", b.Val)}
	}

	return Val{Val: a.Val[b.Val]}
}

func IndexMap[Val ~struct{Val V; Err error}, K comparable, V any, KVal ~struct{Val K; Err error}](a MapValue[K, V], b KVal) Val {
	bStruct := (struct{Val K; Err error})(b)
	if a.Err != nil {
		return Val{Err: a.Err}
	}
	if bStruct.Err != nil {
		return Val{Err: bStruct.Err}
	}

	elem, ok := a.Val[bStruct.Val]
	if ok {
		return Val{Val: elem}
	}

	// Handle numeric equality for int and uint types as map keys.
	switch b := any(bStruct.Val).(type) {
	case int64:
		if b >= 0 {
			switch a := any(a.Val).(type) {
			case map[any]V:
				if elem, ok := a[uint64(b)]; ok {
					return Val{Val: elem}
				}
			}
		}
	case uint64:
		if b <= math.MaxInt64 {
			switch a := any(a.Val).(type) {
			case map[any]V:
				if elem, ok := a[int64(b)]; ok {
					return Val{Val: elem}
				}
			}
		}
	}

	return Val{Err: fmt.Errorf("no such key %v", bStruct.Val)}
}

func InList[T any, U any, Val ~struct{Val U; Err error}](a Val, b ListValue[T]) BoolValue {
	aStruct := (struct{Val U; Err error})(a)
	if aStruct.Err != nil {
		return BoolValue{Err: aStruct.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	for _, elem := range b.Val {
		if Eq(elem, aStruct.Val) {
			return BoolValue{Val: true}
		}
	}
	return BoolValue{Val: false}
}

func InMap[K comparable, V any, KVal ~struct{Val K; Err error}](a KVal, b MapValue[K, V]) BoolValue {
	aStruct := (struct{Val K; Err error})(a)
	if aStruct.Err != nil {
		return BoolValue{Err: aStruct.Err}
	}
	if b.Err != nil {
		return BoolValue{Err: b.Err}
	}
	if _, ok := b.Val[aStruct.Val]; ok {
		return BoolValue{Val: true}
	}

	// Handle numeric equality for int and uint types as map keys.
	switch a := any(aStruct.Val).(type) {
	case int64:
		if a >= 0 {
			switch b := any(b.Val).(type) {
			case map[any]V:
				if _, ok := b[uint64(a)]; ok {
					return BoolValue{Val: true}
				}
			}
		}
	case uint64:
		if a <= math.MaxInt64 {
			switch b := any(b.Val).(type) {
			case map[any]V:
				if _, ok := b[int64(a)]; ok {
					return BoolValue{Val: true}
				}
			}
		}
	}

	return BoolValue{Val: false}
}
