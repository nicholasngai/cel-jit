package runtime

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

//
// OPERATORS.
//

func Select(a DynValue, fieldName string) DynValue {
	if a.Err != nil {
		return a
	}

	aVal := reflect.ValueOf(a.Val)
	if aVal.Type().Kind() != reflect.Map {
		return DynValue{Err: errors.New("not a map")}
	}

	if !reflect.TypeFor[string]().AssignableTo(aVal.Type().Key()) {
		return DynValue{Err: fmt.Errorf("no such key %q", fieldName)}
	}

	elemVal := aVal.MapIndex(reflect.ValueOf(fieldName))
	if !elemVal.IsValid() {
		return DynValue{Err: fmt.Errorf("no such key %q", fieldName)}
	}

	return DynValue{Val: elemVal.Interface()}
}

func Has(a DynValue, fieldName string) DynValue {
	if a.Err != nil {
		return a
	}

	aVal := reflect.ValueOf(a.Val)
	if aVal.Type().Kind() != reflect.Map {
		return DynValue{Val: false}
	}

	if !reflect.TypeFor[string]().AssignableTo(aVal.Type().Key()) {
		return DynValue{Val: false}
	}

	return DynValue{Val: aVal.MapIndex(reflect.ValueOf(fieldName)).IsValid()}
}

func LogicalAnd(a, b DynValue) DynValue {
	// Unlike most other operators, logical AND may swallow errors if either
	// input is false.
	aBool, aOk := a.Val.(bool)
	bBool, bOk := b.Val.(bool)
	if aOk && !aBool || bOk && !bBool {
		return DynValue{Val: false}
	}

	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	if aOk && bOk {
		return DynValue{Val: true}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func LogicalOr(a, b DynValue) DynValue {
	// Unlike most other operators, logical OR may swallow errors if either
	// input is true.
	aBool, aOk := a.Val.(bool)
	bBool, bOk := b.Val.(bool)
	if aOk && aBool || bOk && bBool {
		return DynValue{Val: true}
	}

	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	if aOk && bOk {
		return DynValue{Val: false}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func LogicalNot(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	aBool, aOk := a.Val.(bool)
	if !aOk {
		return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
	}

	return DynValue{Val: !aBool}
}

func Equals(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	return DynValue{Val: eq(a.Val, b.Val)}
}

func NotEquals(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	return DynValue{Val: !eq(a.Val, b.Val)}
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
	case []struct{}:
		if b, ok := b.([]time.Time); ok {
			return len(a) == len(b)
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
			if aType.Key().AssignableTo(bType.Key()) {
				bElemVal := bVal.MapIndex(aMapIter.Key())
				if bElemVal.IsValid() {
					// Found the element. Check if it's equal.
					if !eq(aMapIter.Value().Interface(), bElemVal.Interface()) {
						return false
					}
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

func Less(a, b DynValue) DynValue {
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

func LessEquals(a, b DynValue) DynValue {
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

func Greater(a, b DynValue) DynValue {
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

func GreaterEquals(a, b DynValue) DynValue {
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
	a, b DynValue,
	cmpInt func(a, b int64) bool,
	cmpUint func(a, b uint64) bool,
	cmpDouble func(a, b float64) bool,
	cmpBool func(a, b bool) bool,
	cmpString func(a, b string) bool,
	cmpBytes func(a, b []byte) bool,
	cmpTime func(a, b time.Time) bool,
	cmpDuration func(a, b time.Duration) bool,
) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aInt, aOk := a.Val.(int64)
	bInt, bOk := b.Val.(int64)
	if aOk && bOk {
		return DynValue{Val: cmpInt(aInt, bInt)}
	}
	aUint, aOk := a.Val.(uint64)
	bUint, bOk := b.Val.(uint64)
	if aOk && bOk {
		return DynValue{Val: cmpUint(aUint, bUint)}
	}
	aDouble, aOk := a.Val.(float64)
	bDouble, bOk := b.Val.(float64)
	if aOk && bOk {
		return DynValue{Val: cmpDouble(aDouble, bDouble)}
	}
	aBool, aOk := a.Val.(bool)
	bBool, bOk := b.Val.(bool)
	if aOk && bOk {
		return DynValue{Val: cmpBool(aBool, bBool)}
	}
	aString, aOk := a.Val.(string)
	bString, bOk := b.Val.(string)
	if aOk && bOk {
		return DynValue{Val: cmpString(aString, bString)}
	}
	aBytes, aOk := a.Val.([]byte)
	bBytes, bOk := b.Val.([]byte)
	if aOk && bOk {
		return DynValue{Val: cmpBytes(aBytes, bBytes)}
	}
	aTime, aOk := a.Val.(time.Time)
	bTime, bOk := b.Val.(time.Time)
	if aOk && bOk {
		return DynValue{Val: cmpTime(aTime, bTime)}
	}
	aDuration, aOk := a.Val.(time.Duration)
	bDuration, bOk := b.Val.(time.Duration)
	if aOk && bOk {
		return DynValue{Val: cmpDuration(aDuration, bDuration)}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func Add(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aInt, aOk := a.Val.(int64)
	bInt, bOk := b.Val.(int64)
	if aOk && bOk {
		return DynValue{Val: aInt + bInt}
	}

	aUint, aOk := a.Val.(uint64)
	bUint, bOk := b.Val.(uint64)
	if aOk && bOk {
		return DynValue{Val: aUint + bUint}
	}

	aDouble, aOk := a.Val.(float64)
	bDouble, bOk := b.Val.(float64)
	if aOk && bOk {
		return DynValue{Val: aDouble + bDouble}
	}

	aStr, aOk := a.Val.(string)
	bStr, bOk := b.Val.(string)
	if aOk && bOk {
		return DynValue{Val: aStr + bStr}
	}

	aTime, aIsTime := a.Val.(time.Time)
	bTime, bIsTime := b.Val.(time.Time)
	aDuration, aIsDuration := a.Val.(time.Duration)
	bDuration, bIsDuration := b.Val.(time.Duration)
	if aIsTime && bIsDuration {
		return DynValue{Val: aTime.Add(bDuration)}
	}
	if aIsDuration && bIsTime {
		return DynValue{Val: bTime.Add(aDuration)}
	}
	if aIsDuration && bIsDuration {
		return DynValue{Val: aDuration + bDuration}
	}

	aVal := reflect.ValueOf(a.Val)
	bVal := reflect.ValueOf(b.Val)
	aType := aVal.Type()
	bType := bVal.Type()
	aIsList := aType.Kind() == reflect.Slice
	bIsList := bType.Kind() == reflect.Slice
	if aIsList && bIsList {
		if aType.Elem() == bType.Elem() {
			return DynValue{Val: reflect.AppendSlice(aVal, bVal).Interface()}
		}

		// Differing types. Fall back to []any.
		res := make([]any, aVal.Len()+bVal.Len())
		for i := range aVal.Len() {
			res[i] = aVal.Index(i).Interface()
		}
		for i := range bVal.Len() {
			res[aVal.Len()+i] = bVal.Index(i).Interface()
		}
		return DynValue{Val: res}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func Subtract(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aInt, aOk := a.Val.(int64)
	bInt, bOk := b.Val.(int64)
	if aOk && bOk {
		return DynValue{Val: aInt - bInt}
	}

	aUint, aOk := a.Val.(uint64)
	bUint, bOk := b.Val.(uint64)
	if aOk && bOk {
		return DynValue{Val: aUint - bUint}
	}

	aDouble, aOk := a.Val.(float64)
	bDouble, bOk := b.Val.(float64)
	if aOk && bOk {
		return DynValue{Val: aDouble - bDouble}
	}

	aTime, aIsTime := a.Val.(time.Time)
	bTime, bIsTime := b.Val.(time.Time)
	aDuration, aIsDuration := a.Val.(time.Duration)
	bDuration, bIsDuration := b.Val.(time.Duration)
	if aIsTime && bIsTime {
		return DynValue{Val: aTime.Sub(bTime)}
	}
	if aIsTime && bIsDuration {
		return DynValue{Val: aTime.Add(-bDuration)}
	}
	if aIsDuration && bIsDuration {
		return DynValue{Val: aDuration - bDuration}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func Multiply(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aInt, aOk := a.Val.(int64)
	bInt, bOk := b.Val.(int64)
	if aOk && bOk {
		return DynValue{Val: aInt * bInt}
	}

	aUint, aOk := a.Val.(uint64)
	bUint, bOk := b.Val.(uint64)
	if aOk && bOk {
		return DynValue{Val: aUint * bUint}
	}

	aDouble, aOk := a.Val.(float64)
	bDouble, bOk := b.Val.(float64)
	if aOk && bOk {
		return DynValue{Val: aDouble * bDouble}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func Divide(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aInt, aOk := a.Val.(int64)
	bInt, bOk := b.Val.(int64)
	if aOk && bOk {
		return DynValue{Val: aInt / bInt}
	}

	aUint, aOk := a.Val.(uint64)
	bUint, bOk := b.Val.(uint64)
	if aOk && bOk {
		return DynValue{Val: aUint / bUint}
	}

	aDouble, aOk := a.Val.(float64)
	bDouble, bOk := b.Val.(float64)
	if aOk && bOk {
		return DynValue{Val: aDouble / bDouble}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func Modulo(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aInt, aOk := a.Val.(int64)
	bInt, bOk := b.Val.(int64)
	if aOk && bOk {
		return DynValue{Val: aInt % bInt}
	}

	aUint, aOk := a.Val.(uint64)
	bUint, bOk := b.Val.(uint64)
	if aOk && bOk {
		return DynValue{Val: aUint % bUint}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func Negate(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	aInt, ok := a.Val.(int64)
	if ok {
		return DynValue{Val: -aInt}
	}

	aDouble, ok := a.Val.(float64)
	if ok {
		return DynValue{Val: -aDouble}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Index(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aVal := reflect.ValueOf(a.Val)
	aType := aVal.Type()
	switch aType.Kind() {
	case reflect.Slice:
		// Turn the numeric value into an integer.
		var index int
		switch bVal := b.Val.(type) {
		case int64:
			index = int(bVal)
		case uint64:
			index = int(bVal)
		case float64:
			if float64(int(bVal)) != bVal {
				return DynValue{Err: fmt.Errorf("cannot index list with value %f", bVal)}
			}
			index = int(bVal)
		default:
			return DynValue{Err: fmt.Errorf("cannot index list with type %T", b.Val)}
		}

		// Check bounds.
		if int(index) < 0 || int(index) >= aVal.Len() {
			return DynValue{Err: fmt.Errorf("index %d out of range", index)}
		}

		return DynValue{Val: aVal.Index(index).Interface()}

	case reflect.Map:
		bVal := reflect.ValueOf(b.Val)
		if bVal.Type().AssignableTo(aVal.Type().Key()) {
			if elemVal := aVal.MapIndex(bVal); elemVal.IsValid() {
				return DynValue{Val: elemVal.Interface()}
			}
		}

		// We didn't find the element. See if we need to do a scan for numeric
		// comparison. Only int and uints can be keys and numerically compared.
		switch b.Val.(type) {
		case int64:
		case uint64:
		default:
			return DynValue{Err: fmt.Errorf("no such key %v", bVal)}
		}

		// If this is a key type that supports numeric equality, then we need to
		// iterate through the keys of b.
		for aMapIter := aVal.MapRange(); aMapIter.Next(); {
			if !eq(b.Val, aMapIter.Key().Interface()) {
				continue
			}

			// Found the element.
			return DynValue{Val: aMapIter.Value().Interface()}
		}

		return DynValue{Err: fmt.Errorf("no such key %v", bVal)}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func NotStrictlyFalse(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	return DynValue{Val: a.Val != false}
}

func In(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aVal := reflect.ValueOf(a.Val)
	bVal := reflect.ValueOf(b.Val)
	bType := bVal.Type()
	switch bType.Kind() {
	case reflect.Slice:
		for i := range bVal.Len() {
			if eq(a.Val, bVal.Index(i).Interface()) {
				return DynValue{Val: true}
			}
		}
		return DynValue{Val: false}

	case reflect.Map:
		if aVal.Type().AssignableTo(bType.Key()) {
			if bVal.MapIndex(aVal).IsValid() {
				return DynValue{Val: true}
			}
		}

		// We didn't find the element. See if we need to do a scan for numeric
		// comparison. Only int and uints can be keys and numerically compared.
		switch a.Val.(type) {
		case int64:
		case uint64:
		default:
			return DynValue{Val: false}
		}

		// If this is a key type that supports numeric equality, then we need to
		// iterate through the keys of b.
		for bMapIter := bVal.MapRange(); bMapIter.Next(); {
			if !eq(a.Val, bMapIter.Key().Interface()) {
				continue
			}

			// Found the element.
			return DynValue{Val: true}
		}

		return DynValue{Val: false}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

//
// OVERLOADS.
//

func Size(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch a := a.Val.(type) {
	case string:
		return DynValue{Val: int64(len(a))}
	case []byte:
		return DynValue{Val: int64(len(a))}
	}

	aVal := reflect.ValueOf(a.Val)
	switch aVal.Type().Kind() {
	case reflect.Slice:
		return DynValue{Val: int64(aVal.Len())}
	case reflect.Map:
		return DynValue{Val: int64(aVal.Len())}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Contains(a, b DynValue) DynValue {
	return eval2(a, b, strings.Contains)
}

func EndsWith(a, b DynValue) DynValue {
	return eval2(a, b, strings.HasSuffix)
}

func Matches(a, b DynValue) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aStr, aOk := a.Val.(string)
	bStr, bOk := b.Val.(string)
	if aOk && bOk {
		// TODO(nngai) Can we pre-compile this somehow?
		re, err := regexp.Compile(bStr)
		if err != nil {
			return DynValue{Err: fmt.Errorf("invalid regex %q: %w", bStr, err)}
		}
		return DynValue{Val: re.MatchString(aStr)}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func StartsWith(a, b DynValue) DynValue {
	return eval2(a, b, strings.HasPrefix)
}

func eval2[T any, U any, R any](a, b DynValue, eval func(a T, b U) R) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	aStr, aOk := a.Val.(T)
	bStr, bOk := b.Val.(U)
	if aOk && bOk {
		return DynValue{Val: eval(aStr, bStr)}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func GetFullYear(a, b DynValue) DynValue {
	return evalTime(a, b, time.Time.Year)
}

func GetMonth(a, b DynValue) DynValue {
	return evalTime(a, b, func(a time.Time) time.Month { return a.Month() - 1 })
}

func GetDayOfYear(a, b DynValue) DynValue {
	return evalTime(a, b, func(a time.Time) int { return a.YearDay() - 1 })
}

func GetDate(a, b DynValue) DynValue {
	return evalTime(a, b, time.Time.Day)
}

func GetDayOfMonth(a, b DynValue) DynValue {
	return evalTime(a, b, func(a time.Time) int { return a.Day() - 1 })
}

func GetDayOfWeek(a, b DynValue) DynValue {
	return evalTime(a, b, time.Time.Weekday)
}

func evalTime[T ~int](a, b DynValue, eval func(a time.Time) T) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	switch a := a.Val.(type) {
	case time.Time:
		switch b := b.Val.(type) {
		case string:
			loc, err := time.LoadLocation(b)
			if err != nil {
				return DynValue{Err: fmt.Errorf("unknown location %q", b)}
			}
			return DynValue{Val: int64(eval(a.In(loc)))}
		case nil:
			return DynValue{Val: int64(eval(a.UTC()))}
		}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

func GetHours(a, b DynValue) DynValue {
	return evalTimeOrDuration(
		a, b,
		time.Time.Hour,
		func(a time.Duration) int64 { return int64(a / time.Hour) },
	)
}

func GetMinutes(a, b DynValue) DynValue {
	return evalTimeOrDuration(
		a, b,
		time.Time.Minute,
		func(a time.Duration) int64 { return int64(a / time.Minute) },
	)
}

func GetSeconds(a, b DynValue) DynValue {
	return evalTimeOrDuration(
		a, b,
		time.Time.Second,
		func(a time.Duration) int64 { return int64(a / time.Second) },
	)
}

func GetMilliseconds(a, b DynValue) DynValue {
	return evalTimeOrDuration(
		a, b,
		func(a time.Time) int { return a.Nanosecond() / 1000000 },
		func(a time.Duration) int64 { return a.Milliseconds() % 1000 },
	)
}

func evalTimeOrDuration[T ~int, U ~int64](a, b DynValue, evalTime func(a time.Time) T, evalDuration func(a time.Duration) U) DynValue {
	if a.Err != nil {
		return a
	}
	if b.Err != nil {
		return b
	}

	switch a := a.Val.(type) {
	case time.Time:
		switch b := b.Val.(type) {
		case string:
			loc, err := time.LoadLocation(b)
			if err != nil {
				return DynValue{Err: fmt.Errorf("unknown location %q", b)}
			}
			return DynValue{Val: int64(evalTime(a.In(loc)))}
		case nil:
			return DynValue{Val: int64(evalTime(a.UTC()))}
		}
	case time.Duration:
		switch b.Val.(type) {
		case nil:
			return DynValue{Val: int64(evalDuration(a))}
		}
	}

	return DynValue{Err: fmt.Errorf("incompatible types %T and %T", a.Val, b.Val)}
}

//
// TYPE CONVERSION.
//

func Int(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case int64:
		return a
	case uint64:
		if aType > uint64(math.MaxInt64) {
			return DynValue{Err: fmt.Errorf("integer overflow %d", aType)}
		}
		return DynValue{Val: int64(aType)}
	case float64:
		if aType > float64(math.MaxInt64) || aType < float64(math.MinInt64) {
			return DynValue{Err: fmt.Errorf("integer overflow %f", aType)}
		}
		return DynValue{Val: int64(aType)}
	case time.Time:
		return DynValue{Val: aType.Unix()}
	case string:
		i, err := strconv.ParseInt(aType, 10, 64)
		if err != nil {
			return DynValue{Err: fmt.Errorf("invalid int %q: %w", aType, err)}
		}
		return DynValue{Val: i}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Uint(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case uint64:
		return a
	case int64:
		if aType < 0 {
			return DynValue{Err: fmt.Errorf("unsigned integer overflow %d", aType)}
		}
		return DynValue{Val: uint64(aType)}
	case float64:
		if aType > float64(math.MaxUint64) || aType < 0 {
			return DynValue{Err: fmt.Errorf("unsigned integer overflow %f", aType)}
		}
		return DynValue{Val: uint64(aType)}
	case string:
		i, err := strconv.ParseUint(aType, 10, 64)
		if err != nil {
			return DynValue{Err: fmt.Errorf("invalid uint %q: %w", aType, err)}
		}
		return DynValue{Val: i}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Double(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case float64:
		return a
	case int64:
		return DynValue{Val: float64(aType)}
	case uint64:
		return DynValue{Val: float64(aType)}
	case string:
		f, err := strconv.ParseFloat(aType, 10)
		if err != nil {
			return DynValue{Err: fmt.Errorf("invalid float %q: %w", aType, err)}
		}
		return DynValue{Val: f}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Bool(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case bool:
		return a
	case string:
		b, err := strconv.ParseBool(aType)
		if err != nil {
			return DynValue{Err: fmt.Errorf("invalid bool %q: %w", aType, err)}
		}
		return DynValue{Val: b}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func String(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case string:
		return a
	case int64:
		return DynValue{Val: strconv.FormatInt(aType, 10)}
	case uint64:
		return DynValue{Val: strconv.FormatUint(aType, 10)}
	case float64:
		return DynValue{Val: strconv.FormatFloat(aType, 'g', -1, 64)}
	case bool:
		return DynValue{Val: strconv.FormatBool(aType)}
	case []byte:
		return DynValue{Val: string(aType)}
	case time.Time:
		return DynValue{Val: aType.Format(time.RFC3339Nano)}
	case time.Duration:
		if aType%time.Second == 0 {
			return DynValue{Val: fmt.Sprintf("%ds", int64(aType/time.Second))}
		} else {
			return DynValue{Val: fmt.Sprintf("%ss", strconv.FormatFloat(aType.Seconds(), 'f', -1, 64))}
		}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Bytes(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case []byte:
		return a
	case string:
		return DynValue{Val: []byte(aType)}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Timestamp(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case time.Time:
		return a
	case string:
		t, err := time.Parse(time.RFC3339Nano, aType)
		if err != nil {
			return DynValue{Err: fmt.Errorf("invalid timestamp %q: %w", aType, err)}
		}
		return DynValue{Val: t}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}

func Duration(a DynValue) DynValue {
	if a.Err != nil {
		return a
	}

	switch aType := a.Val.(type) {
	case time.Duration:
		return a
	case string:
		d, err := time.ParseDuration(aType)
		if err != nil {
			return DynValue{Err: fmt.Errorf("invalid duration %q: %w", aType, err)}
		}
		return DynValue{Val: d}
	}

	return DynValue{Err: fmt.Errorf("incompatible type %T", a.Val)}
}
