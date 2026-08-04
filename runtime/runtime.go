package runtime

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"reflect"
	"slices"
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

// ValueOf returns a [Value] for the given value.
func ValueOf(v any) Value {
	return Value{v: v}
}

// ValueOfSlice returns a [Value] containing a slice from an iterator of
// [Value]s. If any value is an error, it returns the first error value
// instead.
func ValueOfSlice(elems iter.Seq[Value], len int) Value {
	listVal := make([]any, 0, len)
	for elem := range elems {
		if elem.err != nil {
			return elem
		}
		listVal = append(listVal, elem.v)
	}
	return Value{v: listVal}
}

// ValueOfMap returns a [Value] containing a map from an iterator of key-value
// [Value] pairs. If any value is an error, it returns the first error value
// instead.
func ValueOfMap(entries iter.Seq2[Value, Value], len int) Value {
	mapVal := make(map[any]any, len)
	for key, value := range entries {
		if key.err != nil {
			return key
		}
		if value.err != nil {
			return value
		}
		mapVal[key.v] = value.v
	}
	return Value{v: mapVal}
}

func ErrorOf(err error) Value {
	return Value{err: err}
}

func Select(a Value, fieldName string) Value {
	if a.err != nil {
		return a
	}

	aVal := reflect.ValueOf(a.v)
	if aVal.Type().Kind() != reflect.Map {
		return ErrorOf(errors.New("not a map"))
	}

	elemVal := aVal.MapIndex(reflect.ValueOf(fieldName))
	if !elemVal.IsValid() {
		return ErrorOf(fmt.Errorf("no such key %q", fieldName))
	}

	return ValueOf(elemVal.Interface())
}

func Has(a Value, fieldName string) Value {
	if a.err != nil {
		return a
	}

	aVal := reflect.ValueOf(a.v)
	if aVal.Type().Kind() != reflect.Map {
		return ValueOf(false)
	}

	return ValueOf(aVal.MapIndex(reflect.ValueOf(fieldName)).IsValid())
}

func LogicalAnd(a, b Value) Value {
	// Unlike most other operators, logical AND may swallow errors if either
	// input is false.
	aBool, aOk := a.v.(bool)
	bBool, bOk := b.v.(bool)
	if aOk && !aBool || bOk && !bBool {
		return ValueOf(false)
	}

	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	if aOk && bOk {
		return ValueOf(true)
	}

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func LogicalOr(a, b Value) Value {
	// Unlike most other operators, logical OR may swallow errors if either
	// input is true.
	aBool, aOk := a.v.(bool)
	bBool, bOk := b.v.(bool)
	if aOk && aBool || bOk && bBool {
		return ValueOf(true)
	}

	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	if aOk && bOk {
		return ValueOf(false)
	}

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func LogicalNot(a Value) Value {
	if a.err != nil {
		return a
	}

	aBool, aOk := a.v.(bool)
	if !aOk {
		return ErrorOf(fmt.Errorf("incompatible type %T", a.v))
	}

	return ValueOf(!aBool)
}

func Equals(a, b Value) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	return ValueOf(eq(a.v, b.v))
}

func NotEquals(a, b Value) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	return ValueOf(!eq(a.v, b.v))
}

func eq(a, b any) bool {
	// Numeric equality.
	aInt, aIsInt := a.(int64)
	bInt, bIsInt := b.(int64)
	aUint, aIsUint := a.(uint64)
	bUint, bIsUint := b.(uint64)
	aDouble, aIsDouble := a.(float64)
	bDouble, bIsDouble := b.(float64)
	switch {
	// Types are the same.
	case aIsInt && bIsInt:
		return aInt == bInt
	case aIsUint && bIsUint:
		return aUint == bUint
	case aIsDouble && bIsDouble:
		return aDouble == bDouble
	// Mismatched integer casts.
	case aIsInt && bIsUint:
		return aInt >= 0 && bUint <= math.MaxInt64 && aInt == int64(bUint)
	case aIsUint && bIsInt:
		return aInt >= 0 && bUint <= math.MaxInt64 && aInt == int64(bUint)
	// One of these is a float. Cast both to floats.
	case aIsInt && bIsDouble:
		return float64(aInt) == bDouble
	case aIsUint && bIsDouble:
		return float64(aUint) == bDouble
	case aIsDouble && bIsInt:
		return aDouble == float64(bInt)
	case aIsDouble && bIsUint:
		return aDouble == float64(bUint)
	}

	// TODO(nngai) Not sure how to do proto equality. Maybe we can just rely on
	// native types.

	// Slice equality. Start with the comparable types and then fall back to
	// reflection.
	switch a := a.(type) {
	case []int64:
		if b, ok := b.([]int64); ok {
			return slices.Equal(a, b)
		}
	case []uint64:
		if b, ok := b.([]uint64); ok {
			return slices.Equal(a, b)
		}
	case []float64:
		if b, ok := b.([]float64); ok {
			return slices.Equal(a, b)
		}
	case []bool:
		if b, ok := b.([]bool); ok {
			return slices.Equal(a, b)
		}
	case []string:
		if b, ok := b.([]string); ok {
			return slices.Equal(a, b)
		}
	case [][]byte:
		if b, ok := b.([][]byte); ok {
			return slices.EqualFunc(a, b, slices.Equal)
		}
	case []time.Duration:
		if b, ok := b.([]time.Duration); ok {
			return slices.Equal(a, b)
		}
	case []time.Time:
		if b, ok := b.([]time.Time); ok {
			return slices.EqualFunc(a, b, time.Time.Equal)
		}
	}

	// Slice equality.
	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)
	aType := aVal.Type()
	bType := bVal.Type()
	aIsList := aType.Kind() == reflect.Slice
	bIsList := bType.Kind() == reflect.Slice
	if aIsList && bIsList {
		if aVal.Len() != bVal.Len() {
			return false
		}
		for i := range aVal.Len() {
			if !eq(aVal.Index(i).Interface(), bVal.Index(i).Interface()) {
				return false
			}
		}
		return true
	}

	// Map equality.
	aIsMap := aType.Kind() == reflect.Map
	bIsMap := bType.Kind() == reflect.Map
	if aIsMap && bIsMap {
		if aVal.Len() != bVal.Len() {
			return false
		}

		for aMapIter := aVal.MapRange(); aMapIter.Next(); {
			bElemVal := bVal.MapIndex(aMapIter.Key())
			if bElemVal.IsValid() {
				// Found the element. Check if it's equal.
				if !eq(aMapIter.Value().Interface(), bElemVal.Interface()) {
					return false
				}
			}

			// We didn't find the element. See if we need to do a scan for
			// numeric comparison. Only int and uints can be keys and
			// numerically compared.
			switch aMapIter.Key().Interface().(type) {
			case int64:
			case uint64:
			default:
				return false
			}

			// If this is a key type that supports numeric equality, then we
			// need to iterate through the keys of b.
			found := false
			for bMapIter := bVal.MapRange(); bMapIter.Next(); {
				if !eq(aMapIter.Key().Interface(), bMapIter.Key().Interface()) {
					continue
				}

				// Found the element.
				found = true
				if !eq(aMapIter.Value().Interface(), bMapIter.Value().Interface()) {
					return false
				}
				break
			}
			if !found {
				return false
			}
		}

		return true
	}

	// Type equality. == in Go will check for type equality.
	return a == b
}

func Less(a, b Value) Value {
	return compare(
		a, b,
		func(a, b int64) bool { return a < b },
		func(a, b uint64) bool { return a < b },
		func(a, b float64) bool { return a < b },
		func(a, b bool) bool { return !a && b },
		func(a, b string) bool { return a < b },
		func(a, b []byte) bool { return slices.Compare(a, b) < 0 },
		func(a, b time.Time) bool { return a.Compare(b) < 0 },
		func(a, b time.Duration) bool { return a < b },
	)
}

func LessEquals(a, b Value) Value {
	return compare(
		a, b,
		func(a, b int64) bool { return a <= b },
		func(a, b uint64) bool { return a <= b },
		func(a, b float64) bool { return a <= b },
		func(a, b bool) bool { return !a || b },
		func(a, b string) bool { return a <= b },
		func(a, b []byte) bool { return slices.Compare(a, b) <= 0 },
		func(a, b time.Time) bool { return a.Compare(b) <= 0 },
		func(a, b time.Duration) bool { return a <= b },
	)
}

func Greater(a, b Value) Value {
	return compare(
		a, b,
		func(a, b int64) bool { return a > b },
		func(a, b uint64) bool { return a > b },
		func(a, b float64) bool { return a > b },
		func(a, b bool) bool { return a && !b },
		func(a, b string) bool { return a > b },
		func(a, b []byte) bool { return slices.Compare(a, b) > 0 },
		func(a, b time.Time) bool { return a.Compare(b) > 0 },
		func(a, b time.Duration) bool { return a > b },
	)
}

func GreaterEquals(a, b Value) Value {
	return compare(
		a, b,
		func(a, b int64) bool { return a >= b },
		func(a, b uint64) bool { return a >= b },
		func(a, b float64) bool { return a >= b },
		func(a, b bool) bool { return a || !b },
		func(a, b string) bool { return a >= b },
		func(a, b []byte) bool { return slices.Compare(a, b) >= 0 },
		func(a, b time.Time) bool { return a.Compare(b) >= 0 },
		func(a, b time.Duration) bool { return a >= b },
	)
}

func compare(
	a, b Value,
	cmpInt func(a, b int64) bool,
	cmpUint func(a, b uint64) bool,
	cmpDouble func(a, b float64) bool,
	cmpBool func(a, b bool) bool,
	cmpString func(a, b string) bool,
	cmpBytes func(a, b []byte) bool,
	cmpTime func(a, b time.Time) bool,
	cmpDuration func(a, b time.Duration) bool,
) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return ValueOf(cmpInt(aInt, bInt))
	}
	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return ValueOf(cmpUint(aUint, bUint))
	}
	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return ValueOf(cmpDouble(aDouble, bDouble))
	}
	aBool, aOk := a.v.(bool)
	bBool, bOk := b.v.(bool)
	if aOk && bOk {
		return ValueOf(cmpBool(aBool, bBool))
	}
	aString, aOk := a.v.(string)
	bString, bOk := b.v.(string)
	if aOk && bOk {
		return ValueOf(cmpString(aString, bString))
	}
	aBytes, aOk := a.v.([]byte)
	bBytes, bOk := b.v.([]byte)
	if aOk && bOk {
		return ValueOf(cmpBytes(aBytes, bBytes))
	}
	aTime, aOk := a.v.(time.Time)
	bTime, bOk := b.v.(time.Time)
	if aOk && bOk {
		return ValueOf(cmpTime(aTime, bTime))
	}
	aDuration, aOk := a.v.(time.Duration)
	bDuration, bOk := b.v.(time.Duration)
	if aOk && bOk {
		return ValueOf(cmpDuration(aDuration, bDuration))
	}

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
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
	bVal := reflect.ValueOf(b.v)
	aType := aVal.Type()
	bType := bVal.Type()
	aIsList := aType.Kind() == reflect.Slice
	bIsList := bType.Kind() == reflect.Slice
	if aIsList && bIsList {
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

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Subtract(a, b Value) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return ValueOf(aInt - bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return ValueOf(aUint - bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return ValueOf(aDouble - bDouble)
	}

	aTime, aIsTime := a.v.(time.Time)
	bTime, bIsTime := b.v.(time.Time)
	aDuration, aIsDuration := a.v.(time.Duration)
	bDuration, bIsDuration := b.v.(time.Duration)
	if aIsTime && bIsTime {
		return ValueOf(aTime.Sub(bTime))
	}
	if aIsTime && bIsDuration {
		return ValueOf(aTime.Add(-bDuration))
	}
	if aIsDuration && bIsDuration {
		return ValueOf(aDuration - bDuration)
	}

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Multiply(a, b Value) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return ValueOf(aInt * bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return ValueOf(aUint * bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return ValueOf(aDouble * bDouble)
	}

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Divide(a, b Value) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return ValueOf(aInt / bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return ValueOf(aUint / bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return ValueOf(aDouble / bDouble)
	}

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Modulo(a, b Value) Value {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return ValueOf(aInt % bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return ValueOf(aUint % bUint)
	}

	return ErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Negate(a Value) Value {
	if a.err != nil {
		return a
	}

	aInt, ok := a.v.(int64)
	if ok {
		return ValueOf(-aInt)
	}

	aDouble, ok := a.v.(float64)
	if ok {
		return ValueOf(-aDouble)
	}

	return ErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func NotStrictlyFalse(a Value) Value {
	if a.err != nil {
		return a
	}

	return ValueOf(a.v != false)
}
