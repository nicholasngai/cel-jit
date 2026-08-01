package runtime

import (
	"fmt"
	"reflect"
	"time"
)

type Value struct {
	v   any
	err error
}

func (v Value) Val() any {
	return v.v
}

func (v Value) Err() error {
	return v.err
}

func ValueOf(v any) Value {
	return Value{v: v}
}

func errorOf(err error) Value {
	return Value{err: err}
}

func Add(a, b Value) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return ValueOf(aInt + bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return ValueOf(aUint + bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return ValueOf(aDouble + bDouble)
	}

	aStr, aOk := a.v.(string)
	bStr, bOk := b.v.(string)
	if aOk && bOk {
		return ValueOf(aStr + bStr)
	}

	aTime, aIsTime := a.v.(time.Time)
	bTime, bIsTime := b.v.(time.Time)
	aDuration, aIsDuration := a.v.(time.Duration)
	bDuration, bIsDuration := b.v.(time.Duration)
	if aIsTime && bIsDuration {
		return ValueOf(aTime.Add(bDuration))
	}
	if aIsDuration && bIsTime {
		return ValueOf(bTime.Add(aDuration))
	}
	if aIsDuration && bIsDuration {
		return ValueOf(aDuration + bDuration)
	}

	aVal := reflect.ValueOf(a.v)
	bVal := reflect.ValueOf(a.v)
	aType := reflect.TypeOf(a.v)
	bType := reflect.TypeOf(b.v)
	aIsList := aType.Kind() == reflect.Slice
	bIsList := bType.Kind() == reflect.Slice
	if aIsList && bIsList {
		// []T + []T -> []T
		if aType.Elem() == bType.Elem() {
			return ValueOf(reflect.AppendSlice(aVal, bVal).Interface())
		}

		// Differing types. Fall back to []any.
		res := make([]any, aVal.Len() + bVal.Len())
		for i := range aVal.Len() {
			res[i] = aVal.Index(i).Interface()
		}
		for i := range bVal.Len() {
			res[aVal.Len() + i] = bVal.Index(i).Interface()
		}
		return ValueOf(res)
	}

	return errorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}
