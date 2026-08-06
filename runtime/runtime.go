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
	if a.err != nil {
		return a
	}

	aVal := reflect.ValueOf(a.v)
	if aVal.Type().Kind() != reflect.Map {
		return DynErrorOf(errors.New("not a map"))
	}

	elemVal := aVal.MapIndex(reflect.ValueOf(fieldName))
	if !elemVal.IsValid() {
		return DynErrorOf(fmt.Errorf("no such key %q", fieldName))
	}

	return DynValueOf(elemVal.Interface())
}

func Has(a DynValue, fieldName string) DynValue {
	if a.err != nil {
		return a
	}

	aVal := reflect.ValueOf(a.v)
	if aVal.Type().Kind() != reflect.Map {
		return DynValueOf(false)
	}

	return DynValueOf(aVal.MapIndex(reflect.ValueOf(fieldName)).IsValid())
}

func LogicalAnd(a, b DynValue) DynValue {
	// Unlike most other operators, logical AND may swallow errors if either
	// input is false.
	aBool, aOk := a.v.(bool)
	bBool, bOk := b.v.(bool)
	if aOk && !aBool || bOk && !bBool {
		return DynValueOf(false)
	}

	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	if aOk && bOk {
		return DynValueOf(true)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func LogicalOr(a, b DynValue) DynValue {
	// Unlike most other operators, logical OR may swallow errors if either
	// input is true.
	aBool, aOk := a.v.(bool)
	bBool, bOk := b.v.(bool)
	if aOk && aBool || bOk && bBool {
		return DynValueOf(true)
	}

	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	if aOk && bOk {
		return DynValueOf(false)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func LogicalNot(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	aBool, aOk := a.v.(bool)
	if !aOk {
		return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
	}

	return DynValueOf(!aBool)
}

func Equals(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	return DynValueOf(eq(a.v, b.v))
}

func NotEquals(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	return DynValueOf(!eq(a.v, b.v))
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
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return DynValueOf(cmpInt(aInt, bInt))
	}
	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return DynValueOf(cmpUint(aUint, bUint))
	}
	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return DynValueOf(cmpDouble(aDouble, bDouble))
	}
	aBool, aOk := a.v.(bool)
	bBool, bOk := b.v.(bool)
	if aOk && bOk {
		return DynValueOf(cmpBool(aBool, bBool))
	}
	aString, aOk := a.v.(string)
	bString, bOk := b.v.(string)
	if aOk && bOk {
		return DynValueOf(cmpString(aString, bString))
	}
	aBytes, aOk := a.v.([]byte)
	bBytes, bOk := b.v.([]byte)
	if aOk && bOk {
		return DynValueOf(cmpBytes(aBytes, bBytes))
	}
	aTime, aOk := a.v.(time.Time)
	bTime, bOk := b.v.(time.Time)
	if aOk && bOk {
		return DynValueOf(cmpTime(aTime, bTime))
	}
	aDuration, aOk := a.v.(time.Duration)
	bDuration, bOk := b.v.(time.Duration)
	if aOk && bOk {
		return DynValueOf(cmpDuration(aDuration, bDuration))
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Add(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return DynValueOf(aInt + bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return DynValueOf(aUint + bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return DynValueOf(aDouble + bDouble)
	}

	aStr, aOk := a.v.(string)
	bStr, bOk := b.v.(string)
	if aOk && bOk {
		return DynValueOf(aStr + bStr)
	}

	aTime, aIsTime := a.v.(time.Time)
	bTime, bIsTime := b.v.(time.Time)
	aDuration, aIsDuration := a.v.(time.Duration)
	bDuration, bIsDuration := b.v.(time.Duration)
	if aIsTime && bIsDuration {
		return DynValueOf(aTime.Add(bDuration))
	}
	if aIsDuration && bIsTime {
		return DynValueOf(bTime.Add(aDuration))
	}
	if aIsDuration && bIsDuration {
		return DynValueOf(aDuration + bDuration)
	}

	aVal := reflect.ValueOf(a.v)
	bVal := reflect.ValueOf(b.v)
	aType := aVal.Type()
	bType := bVal.Type()
	aIsList := aType.Kind() == reflect.Slice
	bIsList := bType.Kind() == reflect.Slice
	if aIsList && bIsList {
		if aType.Elem() == bType.Elem() {
			return DynValueOf(reflect.AppendSlice(aVal, bVal).Interface())
		}

		// Differing types. Fall back to []any.
		res := make([]any, aVal.Len()+bVal.Len())
		for i := range aVal.Len() {
			res[i] = aVal.Index(i).Interface()
		}
		for i := range bVal.Len() {
			res[aVal.Len()+i] = bVal.Index(i).Interface()
		}
		return DynValueOf(res)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Subtract(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return DynValueOf(aInt - bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return DynValueOf(aUint - bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return DynValueOf(aDouble - bDouble)
	}

	aTime, aIsTime := a.v.(time.Time)
	bTime, bIsTime := b.v.(time.Time)
	aDuration, aIsDuration := a.v.(time.Duration)
	bDuration, bIsDuration := b.v.(time.Duration)
	if aIsTime && bIsTime {
		return DynValueOf(aTime.Sub(bTime))
	}
	if aIsTime && bIsDuration {
		return DynValueOf(aTime.Add(-bDuration))
	}
	if aIsDuration && bIsDuration {
		return DynValueOf(aDuration - bDuration)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Multiply(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return DynValueOf(aInt * bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return DynValueOf(aUint * bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return DynValueOf(aDouble * bDouble)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Divide(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return DynValueOf(aInt / bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return DynValueOf(aUint / bUint)
	}

	aDouble, aOk := a.v.(float64)
	bDouble, bOk := b.v.(float64)
	if aOk && bOk {
		return DynValueOf(aDouble / bDouble)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Modulo(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aInt, aOk := a.v.(int64)
	bInt, bOk := b.v.(int64)
	if aOk && bOk {
		return DynValueOf(aInt % bInt)
	}

	aUint, aOk := a.v.(uint64)
	bUint, bOk := b.v.(uint64)
	if aOk && bOk {
		return DynValueOf(aUint % bUint)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func Negate(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	aInt, ok := a.v.(int64)
	if ok {
		return DynValueOf(-aInt)
	}

	aDouble, ok := a.v.(float64)
	if ok {
		return DynValueOf(-aDouble)
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Index(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aVal := reflect.ValueOf(a.v)
	aType := aVal.Type()
	switch aType.Kind() {
	case reflect.Slice:
		// Turn the numeric value into an integer.
		var index int
		switch bVal := b.v.(type) {
		case int64:
			index = int(bVal)
		case uint64:
			index = int(bVal)
		case float64:
			if float64(int(bVal)) != bVal {
				return DynErrorOf(fmt.Errorf("cannot index list with value %f", bVal))
			}
			index = int(bVal)
		default:
			return DynErrorOf(fmt.Errorf("cannot index list with type %T", b.v))
		}

		// Check bounds.
		if int(index) < 0 || int(index) >= aVal.Len() {
			return DynErrorOf(fmt.Errorf("index %d out of range", index))
		}

		return DynValueOf(aVal.Index(index).Interface())

	case reflect.Map:
		bVal := reflect.ValueOf(b.v)
		if elemVal := aVal.MapIndex(bVal); elemVal.IsValid() {
			return DynValueOf(elemVal.Interface())
		}

		// We didn't find the element. See if we need to do a scan for numeric
		// comparison. Only int and uints can be keys and numerically compared.
		switch b.v.(type) {
		case int64:
		case uint64:
		default:
			return DynErrorOf(fmt.Errorf("no such key %v", bVal))
		}

		// If this is a key type that supports numeric equality, then we need to
		// iterate through the keys of b.
		for aMapIter := aVal.MapRange(); aMapIter.Next(); {
			if !eq(b.v, aMapIter.Key().Interface()) {
				continue
			}

			// Found the element.
			return DynValueOf(aMapIter.Value().Interface())
		}

		return DynErrorOf(fmt.Errorf("no such key %v", bVal))
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func NotStrictlyFalse(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	return DynValueOf(a.v != false)
}

func In(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aVal := reflect.ValueOf(a.v)
	bVal := reflect.ValueOf(b.v)
	bType := bVal.Type()
	switch bType.Kind() {
	case reflect.Slice:
		for i := range bVal.Len() {
			if eq(a.v, bVal.Index(i).Interface()) {
				return DynValueOf(true)
			}
		}
		return DynValueOf(false)

	case reflect.Map:
		if bVal.MapIndex(aVal).IsValid() {
			return DynValueOf(true)
		}

		// We didn't find the element. See if we need to do a scan for numeric
		// comparison. Only int and uints can be keys and numerically compared.
		switch a.v.(type) {
		case int64:
		case uint64:
		default:
			return DynValueOf(false)
		}

		// If this is a key type that supports numeric equality, then we need to
		// iterate through the keys of b.
		for bMapIter := bVal.MapRange(); bMapIter.Next(); {
			if !eq(a.v, bMapIter.Key().Interface()) {
				continue
			}

			// Found the element.
			return DynValueOf(true)
		}

		return DynValueOf(false)
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

//
// OVERLOADS.
//

func Size(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch a := a.v.(type) {
	case string:
		return DynValueOf(int64(len(a)))
	case []byte:
		return DynValueOf(int64(len(a)))
	}

	aVal := reflect.ValueOf(a.v)
	switch aVal.Type().Kind() {
	case reflect.Slice:
		return DynValueOf(int64(aVal.Len()))
	case reflect.Map:
		return DynValueOf(int64(aVal.Len()))
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Contains(a, b DynValue) DynValue {
	return eval2(a, b, strings.Contains)
}

func EndsWith(a, b DynValue) DynValue {
	return eval2(a, b, strings.HasSuffix)
}

func Matches(a, b DynValue) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aStr, aOk := a.v.(string)
	bStr, bOk := b.v.(string)
	if aOk && bOk {
		// TODO(nngai) Can we pre-compile this somehow?
		re, err := regexp.Compile(bStr)
		if err != nil {
			return DynErrorOf(fmt.Errorf("invalid regex %q: %w", bStr, err))
		}
		return DynValueOf(re.MatchString(aStr))
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

func StartsWith(a, b DynValue) DynValue {
	return eval2(a, b, strings.HasPrefix)
}

func eval2[T any, U any, R any](a, b DynValue, eval func(a T, b U) R) DynValue {
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	aStr, aOk := a.v.(T)
	bStr, bOk := b.v.(U)
	if aOk && bOk {
		return DynValueOf(eval(aStr, bStr))
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
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
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	switch a := a.v.(type) {
	case time.Time:
		switch b := b.v.(type) {
		case string:
			loc, err := time.LoadLocation(b)
			if err != nil {
				return DynErrorOf(fmt.Errorf("unknown location %q", b))
			}
			return DynValueOf(int64(eval(a.In(loc))))
		case nil:
			return DynValueOf(int64(eval(a.UTC())))
		}
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
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
	if a.err != nil {
		return a
	}
	if b.err != nil {
		return b
	}

	switch a := a.v.(type) {
	case time.Time:
		switch b := b.v.(type) {
		case string:
			loc, err := time.LoadLocation(b)
			if err != nil {
				return DynErrorOf(fmt.Errorf("unknown location %q", b))
			}
			return DynValueOf(int64(evalTime(a.In(loc))))
		case nil:
			return DynValueOf(int64(evalTime(a.UTC())))
		}
	case time.Duration:
		switch b.v.(type) {
		case nil:
			return DynValueOf(int64(evalDuration(a)))
		}
	}

	return DynErrorOf(fmt.Errorf("incompatible types %T and %T", a.v, b.v))
}

//
// TYPE CONVERSION.
//

func Int(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case int64:
		return a
	case uint64:
		if aType > uint64(math.MaxInt64) {
			return DynErrorOf(fmt.Errorf("integer overflow %d", aType))
		}
		return DynValueOf(int64(aType))
	case float64:
		if aType > float64(math.MaxInt64) || aType < float64(math.MinInt64) {
			return DynErrorOf(fmt.Errorf("integer overflow %f", aType))
		}
		return DynValueOf(int64(aType))
	case time.Time:
		return DynValueOf(aType.Unix())
	case string:
		i, err := strconv.ParseInt(aType, 10, 64)
		if err != nil {
			return DynErrorOf(fmt.Errorf("invalid int %q: %w", aType, err))
		}
		return DynValueOf(i)
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Uint(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case uint64:
		return a
	case int64:
		if aType < 0 {
			return DynErrorOf(fmt.Errorf("unsigned integer overflow %d", aType))
		}
		return DynValueOf(uint64(aType))
	case float64:
		if aType > float64(math.MaxUint64) || aType < 0 {
			return DynErrorOf(fmt.Errorf("unsigned integer overflow %f", aType))
		}
		return DynValueOf(uint64(aType))
	case string:
		i, err := strconv.ParseUint(aType, 10, 64)
		if err != nil {
			return DynErrorOf(fmt.Errorf("invalid uint %q: %w", aType, err))
		}
		return DynValueOf(i)
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Double(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case float64:
		return a
	case int64:
		return DynValueOf(float64(aType))
	case uint64:
		return DynValueOf(float64(aType))
	case string:
		f, err := strconv.ParseFloat(aType, 10)
		if err != nil {
			return DynErrorOf(fmt.Errorf("invalid float %q: %w", aType, err))
		}
		return DynValueOf(f)
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Bool(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case bool:
		return a
	case string:
		b, err := strconv.ParseBool(aType)
		if err != nil {
			return DynErrorOf(fmt.Errorf("invalid bool %q: %w", aType, err))
		}
		return DynValueOf(b)
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func String(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case string:
		return a
	case int64:
		return DynValueOf(strconv.FormatInt(aType, 10))
	case uint64:
		return DynValueOf(strconv.FormatUint(aType, 10))
	case float64:
		return DynValueOf(strconv.FormatFloat(aType, 'g', -1, 64))
	case bool:
		return DynValueOf(strconv.FormatBool(aType))
	case []byte:
		return DynValueOf(string(aType))
	case time.Time:
		return DynValueOf(aType.Format(time.RFC3339Nano))
	case time.Duration:
		if aType%time.Second == 0 {
			return DynValueOf(fmt.Sprintf("%ds", int64(aType/time.Second)))
		} else {
			return DynValueOf(fmt.Sprintf("%ss", strconv.FormatFloat(aType.Seconds(), 'f', -1, 64)))
		}
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Bytes(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case []byte:
		return a
	case string:
		return DynValueOf([]byte(aType))
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Timestamp(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case time.Time:
		return a
	case string:
		t, err := time.Parse(time.RFC3339Nano, aType)
		if err != nil {
			return DynErrorOf(fmt.Errorf("invalid timestamp %q: %w", aType, err))
		}
		return DynValueOf(t)
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}

func Duration(a DynValue) DynValue {
	if a.err != nil {
		return a
	}

	switch aType := a.v.(type) {
	case time.Duration:
		return a
	case string:
		d, err := time.ParseDuration(aType)
		if err != nil {
			return DynErrorOf(fmt.Errorf("invalid duration %q: %w", aType, err))
		}
		return DynValueOf(d)
	}

	return DynErrorOf(fmt.Errorf("incompatible type %T", a.v))
}
