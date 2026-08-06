package runtime

import "errors"

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
