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
	"sync"
	"time"
)

//
// OPERATORS.
//

func Select(a any, fieldName string) (any, error) {
	aVal := reflect.ValueOf(a)
	if aVal.Type().Kind() != reflect.Map {
		return nil, errors.New("not a map")
	}

	if !reflect.TypeFor[string]().AssignableTo(aVal.Type().Key()) {
		return nil, fmt.Errorf("no such key %q", fieldName)
	}

	elemVal := aVal.MapIndex(reflect.ValueOf(fieldName))
	if !elemVal.IsValid() {
		return nil, fmt.Errorf("no such key %q", fieldName)
	}

	return elemVal.Interface(), nil
}

func Has(a any, fieldName string) bool {
	aVal := reflect.ValueOf(a)
	if aVal.Type().Kind() != reflect.Map {
		return false
	}

	if !reflect.TypeFor[string]().AssignableTo(aVal.Type().Key()) {
		return false
	}

	return aVal.MapIndex(reflect.ValueOf(fieldName)).IsValid()
}

func Equals(a, b any) bool {
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
			if !Equals(aVal.Index(i).Interface(), bVal.Index(i).Interface()) {
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
					if !Equals(aMapIter.Value().Interface(), bElemVal.Interface()) {
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
				if !Equals(aMapIter.Key().Interface(), bMapIter.Key().Interface()) {
					continue
				}

				// Found the element.
				found = true
				if !Equals(aMapIter.Value().Interface(), bMapIter.Value().Interface()) {
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

func NotEquals(a, b any) bool {
	return !Equals(a, b)
}

func Less(a, b any) (bool, error) {
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

func LessEquals(a, b any) (bool, error) {
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

func Greater(a, b any) (bool, error) {
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

func GreaterEquals(a, b any) (bool, error) {
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
	a, b any,
	cmpInt func(a, b int64) bool,
	cmpUint func(a, b uint64) bool,
	cmpDouble func(a, b float64) bool,
	cmpBool func(a, b bool) bool,
	cmpString func(a, b string) bool,
	cmpBytes func(a, b []byte) bool,
	cmpTime func(a, b time.Time) bool,
	cmpDuration func(a, b time.Duration) bool,
) (bool, error) {
	aInt, aOk := a.(int64)
	bInt, bOk := b.(int64)
	if aOk && bOk {
		return cmpInt(aInt, bInt), nil
	}
	aUint, aOk := a.(uint64)
	bUint, bOk := b.(uint64)
	if aOk && bOk {
		return cmpUint(aUint, bUint), nil
	}
	aDouble, aOk := a.(float64)
	bDouble, bOk := b.(float64)
	if aOk && bOk {
		return cmpDouble(aDouble, bDouble), nil
	}
	aBool, aOk := a.(bool)
	bBool, bOk := b.(bool)
	if aOk && bOk {
		return cmpBool(aBool, bBool), nil
	}
	aString, aOk := a.(string)
	bString, bOk := b.(string)
	if aOk && bOk {
		return cmpString(aString, bString), nil
	}
	aBytes, aOk := a.([]byte)
	bBytes, bOk := b.([]byte)
	if aOk && bOk {
		return cmpBytes(aBytes, bBytes), nil
	}
	aTime, aOk := a.(time.Time)
	bTime, bOk := b.(time.Time)
	if aOk && bOk {
		return cmpTime(aTime, bTime), nil
	}
	aDuration, aOk := a.(time.Duration)
	bDuration, bOk := b.(time.Duration)
	if aOk && bOk {
		return cmpDuration(aDuration, bDuration), nil
	}

	return false, fmt.Errorf("incompatible types %T and %T", a, b)
}

func Add(a, b any) (any, error) {
	aInt, aOk := a.(int64)
	bInt, bOk := b.(int64)
	if aOk && bOk {
		return aInt + bInt, nil
	}

	aUint, aOk := a.(uint64)
	bUint, bOk := b.(uint64)
	if aOk && bOk {
		return aUint + bUint, nil
	}

	aDouble, aOk := a.(float64)
	bDouble, bOk := b.(float64)
	if aOk && bOk {
		return aDouble + bDouble, nil
	}

	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		return aStr + bStr, nil
	}

	aTime, aIsTime := a.(time.Time)
	bTime, bIsTime := b.(time.Time)
	aDuration, aIsDuration := a.(time.Duration)
	bDuration, bIsDuration := b.(time.Duration)
	if aIsTime && bIsDuration {
		return aTime.Add(bDuration), nil
	}
	if aIsDuration && bIsTime {
		return bTime.Add(aDuration), nil
	}
	if aIsDuration && bIsDuration {
		return aDuration + bDuration, nil
	}

	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)
	aType := aVal.Type()
	bType := bVal.Type()
	aIsList := aType.Kind() == reflect.Slice
	bIsList := bType.Kind() == reflect.Slice
	if aIsList && bIsList {
		if aType.Elem() == bType.Elem() {
			return reflect.AppendSlice(aVal, bVal).Interface(), nil
		}

		// Differing types. Fall back to []any.
		res := make([]any, aVal.Len()+bVal.Len())
		for i := range aVal.Len() {
			res[i] = aVal.Index(i).Interface()
		}
		for i := range bVal.Len() {
			res[aVal.Len()+i] = bVal.Index(i).Interface()
		}
		return res, nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func Subtract(a, b any) (any, error) {
	aInt, aOk := a.(int64)
	bInt, bOk := b.(int64)
	if aOk && bOk {
		return aInt - bInt, nil
	}

	aUint, aOk := a.(uint64)
	bUint, bOk := b.(uint64)
	if aOk && bOk {
		return aUint - bUint, nil
	}

	aDouble, aOk := a.(float64)
	bDouble, bOk := b.(float64)
	if aOk && bOk {
		return aDouble - bDouble, nil
	}

	aTime, aIsTime := a.(time.Time)
	bTime, bIsTime := b.(time.Time)
	aDuration, aIsDuration := a.(time.Duration)
	bDuration, bIsDuration := b.(time.Duration)
	if aIsTime && bIsTime {
		return aTime.Sub(bTime), nil
	}
	if aIsTime && bIsDuration {
		return aTime.Add(-bDuration), nil
	}
	if aIsDuration && bIsDuration {
		return aDuration - bDuration, nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func Multiply(a, b any) (any, error) {
	aInt, aOk := a.(int64)
	bInt, bOk := b.(int64)
	if aOk && bOk {
		return aInt * bInt, nil
	}

	aUint, aOk := a.(uint64)
	bUint, bOk := b.(uint64)
	if aOk && bOk {
		return aUint * bUint, nil
	}

	aDouble, aOk := a.(float64)
	bDouble, bOk := b.(float64)
	if aOk && bOk {
		return aDouble * bDouble, nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func Divide(a, b any) (any, error) {
	aInt, aOk := a.(int64)
	bInt, bOk := b.(int64)
	if aOk && bOk {
		if bInt == 0 {
			return nil, errors.New("divide by 0")
		}
		return aInt / bInt, nil
	}

	aUint, aOk := a.(uint64)
	bUint, bOk := b.(uint64)
	if aOk && bOk {
		if bUint == 0 {
			return nil, errors.New("divide by 0")
		}
		return aUint / bUint, nil
	}

	aDouble, aOk := a.(float64)
	bDouble, bOk := b.(float64)
	if aOk && bOk {
		return aDouble / bDouble, nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func Modulo(a, b any) (any, error) {
	aInt, aOk := a.(int64)
	bInt, bOk := b.(int64)
	if aOk && bOk {
		if bInt == 0 {
			return nil, errors.New("modulo by 0")
		}
		return aInt % bInt, nil
	}

	aUint, aOk := a.(uint64)
	bUint, bOk := b.(uint64)
	if aOk && bOk {
		if bUint == 0 {
			return nil, errors.New("modulo by 0")
		}
		return aUint % bUint, nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func Negate(a any) (any, error) {
	aInt, ok := a.(int64)
	if ok {
		return -aInt, nil
	}

	aDouble, ok := a.(float64)
	if ok {
		return -aDouble, nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Index(a, b any) (any, error) {
	aVal := reflect.ValueOf(a)
	aType := aVal.Type()
	switch aType.Kind() {
	case reflect.Slice:
		// Turn the numeric value into an integer.
		var index int
		switch bVal := b.(type) {
		case int64:
			index = int(bVal)
		case uint64:
			index = int(bVal)
		case float64:
			if float64(int(bVal)) != bVal {
				return nil, fmt.Errorf("cannot index list with value %f", bVal)
			}
			index = int(bVal)
		default:
			return nil, fmt.Errorf("cannot index list with type %T", b)
		}

		// Check bounds.
		if int(index) < 0 || int(index) >= aVal.Len() {
			return nil, fmt.Errorf("index %d out of range", index)
		}

		return aVal.Index(index).Interface(), nil

	case reflect.Map:
		bVal := reflect.ValueOf(b)
		if bVal.Type().AssignableTo(aVal.Type().Key()) {
			if elemVal := aVal.MapIndex(bVal); elemVal.IsValid() {
				return elemVal.Interface(), nil
			}
		}

		// We didn't find the element. If this is an int, we need to also check
		// the uint, and vice-versa.
		switch b := b.(type) {
		case int64:
			if b > 0 && reflect.TypeFor[uint64]().AssignableTo(aType.Key()) {
				if elemVal := aVal.MapIndex(reflect.ValueOf(uint64(b))); elemVal.IsValid() {
					return elemVal.Interface(), nil
				}
			}
		case uint64:
			if b <= math.MaxInt64 && reflect.TypeFor[int64]().AssignableTo(aType.Key()) {
				if elemVal := aVal.MapIndex(reflect.ValueOf(int64(b))); elemVal.IsValid() {
					return elemVal.Interface(), nil
				}
			}
		}

		return nil, fmt.Errorf("no such key %v", bVal)
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func NotStrictlyFalse(a bool) (bool, error) {
	return a, nil
}

func In(a, b any) (any, error) {
	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)
	bType := bVal.Type()
	switch bType.Kind() {
	case reflect.Slice:
		for i := range bVal.Len() {
			if Equals(a, bVal.Index(i).Interface()) {
				return true, nil
			}
		}
		return false, nil

	case reflect.Map:
		if aVal.Type().AssignableTo(bType.Key()) {
			if bVal.MapIndex(aVal).IsValid() {
				return true, nil
			}
		}

		// We didn't find the element. If this is an int, we need to also check
		// the uint, and vice-versa.
		switch a := a.(type) {
		case int64:
			if a > 0 && reflect.TypeFor[uint64]().AssignableTo(bType.Key()) {
				if bVal.MapIndex(reflect.ValueOf(uint64(a))).IsValid() {
					return true, nil
				}
			}
		case uint64:
			if a <= math.MaxInt64 && reflect.TypeFor[int64]().AssignableTo(bType.Key()) {
				if bVal.MapIndex(reflect.ValueOf(int64(a))).IsValid() {
					return true, nil
				}
			}
		}

		return false, nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

//
// OVERLOADS.
//

func Size(a any) (any, error) {
	switch a := a.(type) {
	case string:
		return int64(len(a)), nil
	case []byte:
		return int64(len(a)), nil
	}

	aVal := reflect.ValueOf(a)
	switch aVal.Type().Kind() {
	case reflect.Slice:
		return int64(aVal.Len()), nil
	case reflect.Map:
		return int64(aVal.Len()), nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Contains(a, b any) (any, error) {
	return eval2(a, b, strings.Contains)
}

func EndsWith(a, b any) (any, error) {
	return eval2(a, b, strings.HasSuffix)
}

func Matches(a, b any) (any, error) {
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		// TODO(nngai) Can we pre-compile this somehow?
		re, err := regexp.Compile(bStr)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", bStr, err)
		}
		return re.MatchString(aStr), nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func StartsWith(a, b any) (any, error) {
	return eval2(a, b, strings.HasPrefix)
}

func eval2[T any, U any, R any](a, b any, eval func(a T, b U) R) (any, error) {
	aStr, aOk := a.(T)
	bStr, bOk := b.(U)
	if aOk && bOk {
		return eval(aStr, bStr), nil
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func GetFullYear(a, b any) (any, error) {
	return evalTime(a, b, time.Time.Year)
}

func GetMonth(a, b any) (any, error) {
	return evalTime(a, b, func(a time.Time) time.Month { return a.Month() - 1 })
}

func GetDayOfYear(a, b any) (any, error) {
	return evalTime(a, b, func(a time.Time) int { return a.YearDay() - 1 })
}

func GetDate(a, b any) (any, error) {
	return evalTime(a, b, time.Time.Day)
}

func GetDayOfMonth(a, b any) (any, error) {
	return evalTime(a, b, func(a time.Time) int { return a.Day() - 1 })
}

func GetDayOfWeek(a, b any) (any, error) {
	return evalTime(a, b, time.Time.Weekday)
}

func GetHours(a, b any) (any, error) {
	return evalTimeOrDuration(
		a, b,
		time.Time.Hour,
		func(a time.Duration) int64 { return int64(a / time.Hour) },
	)
}

func GetMinutes(a, b any) (any, error) {
	return evalTimeOrDuration(
		a, b,
		time.Time.Minute,
		func(a time.Duration) int64 { return int64(a / time.Minute) },
	)
}

func GetSeconds(a, b any) (any, error) {
	return evalTimeOrDuration(
		a, b,
		time.Time.Second,
		func(a time.Duration) int64 { return int64(a / time.Second) },
	)
}

func GetMilliseconds(a, b any) (any, error) {
	return evalTimeOrDuration(
		a, b,
		func(a time.Time) int { return a.Nanosecond() / 1000000 },
		func(a time.Duration) int64 { return a.Milliseconds() % 1000 },
	)
}

func evalTime[T ~int](a, b any, eval func(a time.Time) T) (any, error) {
	switch a := a.(type) {
	case time.Time:
		switch b := b.(type) {
		case string:
			loc, err := loadLocation(b)
			if err != nil {
				return nil, fmt.Errorf("unknown location %q", b)
			}
			return int64(eval(a.In(loc))), nil
		case nil:
			return int64(eval(a.UTC())), nil
		}
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

func evalTimeOrDuration[T ~int, U ~int64](a, b any, evalTime func(a time.Time) T, evalDuration func(a time.Duration) U) (any, error) {
	switch a := a.(type) {
	case time.Time:
		switch b := b.(type) {
		case string:
			loc, err := loadLocation(b)
			if err != nil {
				return nil, fmt.Errorf("unknown location %q", b)
			}
			return int64(evalTime(a.In(loc))), nil
		case nil:
			return int64(evalTime(a.UTC())), nil
		}
	case time.Duration:
		switch b.(type) {
		case nil:
			return int64(evalDuration(a)), nil
		}
	}

	return nil, fmt.Errorf("incompatible types %T and %T", a, b)
}

var locationCache sync.Map

func loadLocation(locString string) (*time.Location, error) {
	if locAny, ok := locationCache.Load(locString); ok {
		return locAny.(*time.Location), nil
	}

	loc, err := time.LoadLocation(locString)
	if err != nil {
		return nil, err
	}

	locationCache.Store(locString, loc)

	return loc, nil
}

//
// TYPE CONVERSION.
//

func Int(a any) (any, error) {
	switch aType := a.(type) {
	case int64:
		return a, nil
	case uint64:
		if aType > uint64(math.MaxInt64) {
			return nil, fmt.Errorf("integer overflow %d", aType)
		}
		return int64(aType), nil
	case float64:
		if aType > float64(math.MaxInt64) || aType < float64(math.MinInt64) {
			return nil, fmt.Errorf("integer overflow %f", aType)
		}
		return int64(aType), nil
	case time.Time:
		return aType.Unix(), nil
	case string:
		i, err := strconv.ParseInt(aType, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int %q: %w", aType, err)
		}
		return i, nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Uint(a any) (any, error) {
	switch aType := a.(type) {
	case uint64:
		return a, nil
	case int64:
		if aType < 0 {
			return nil, fmt.Errorf("unsigned integer overflow %d", aType)
		}
		return uint64(aType), nil
	case float64:
		if aType > float64(math.MaxUint64) || aType < 0 {
			return nil, fmt.Errorf("unsigned integer overflow %f", aType)
		}
		return uint64(aType), nil
	case string:
		i, err := strconv.ParseUint(aType, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid uint %q: %w", aType, err)
		}
		return i, nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Double(a any) (any, error) {
	switch aType := a.(type) {
	case float64:
		return a, nil
	case int64:
		return float64(aType), nil
	case uint64:
		return float64(aType), nil
	case string:
		f, err := strconv.ParseFloat(aType, 10)
		if err != nil {
			return nil, fmt.Errorf("invalid float %q: %w", aType, err)
		}
		return f, nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Bool(a any) (any, error) {
	switch aType := a.(type) {
	case bool:
		return a, nil
	case string:
		b, err := strconv.ParseBool(aType)
		if err != nil {
			return nil, fmt.Errorf("invalid bool %q: %w", aType, err)
		}
		return b, nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func String(a any) (any, error) {
	switch aType := a.(type) {
	case string:
		return a, nil
	case int64:
		return strconv.FormatInt(aType, 10), nil
	case uint64:
		return strconv.FormatUint(aType, 10), nil
	case float64:
		return strconv.FormatFloat(aType, 'g', -1, 64), nil
	case bool:
		return strconv.FormatBool(aType), nil
	case []byte:
		return string(aType), nil
	case time.Time:
		return aType.Format(time.RFC3339Nano), nil
	case time.Duration:
		if aType%time.Second == 0 {
			return fmt.Sprintf("%ds", int64(aType/time.Second)), nil
		} else {
			return fmt.Sprintf("%ss", strconv.FormatFloat(aType.Seconds(), 'f', -1, 64)), nil
		}
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Bytes(a any) (any, error) {
	switch aType := a.(type) {
	case []byte:
		return a, nil
	case string:
		return []byte(aType), nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Timestamp(a any) (any, error) {
	switch aType := a.(type) {
	case time.Time:
		return a, nil
	case string:
		t, err := time.Parse(time.RFC3339Nano, aType)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp %q: %w", aType, err)
		}
		return t, nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}

func Duration(a any) (any, error) {
	switch aType := a.(type) {
	case time.Duration:
		return a, nil
	case string:
		d, err := time.ParseDuration(aType)
		if err != nil {
			return nil, fmt.Errorf("invalid duration %q: %w", aType, err)
		}
		return d, nil
	}

	return nil, fmt.Errorf("incompatible type %T", a)
}
