package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"time"
)

func SelectMap[V any](a map[string]V, fieldName string) (V, error) {
	elem, ok := a[fieldName]
	if !ok {
		var zeroV V
		return zeroV, fmt.Errorf("no such key %v", fieldName)
	}
	return elem, nil
}

func HasMap[V any](a map[string]V, fieldName string) bool {
	_, ok := a[fieldName]
	return ok
}

func LessInt64(a, b int64) bool {
	return a < b
}

func LessInt64Uint64(a int64, b uint64) bool {
	return a < 0 || uint64(a) < b
}

func LessInt64Double(a int64, b float64) bool {
	return float64(a) < b
}

func LessUint64(a, b uint64) bool {
	return a < b
}

func LessUint64Int64(a uint64, b int64) bool {
	return b >= 0 && a < uint64(b)
}

func LessUint64Double(a uint64, b float64) bool {
	return float64(a) < b
}

func LessDouble(a, b float64) bool {
	return a < b
}

func LessDoubleInt64(a float64, b int64) bool {
	return a < float64(b)
}

func LessDoubleUint64(a float64, b uint64) bool {
	return a < float64(b)
}

func LessBool(a, b bool) bool {
	return !a && b
}

func LessString(a, b string) bool {
	return a < b
}

func LessBytes(a, b []byte) bool {
	return bytes.Compare(a, b) < 0
}

func LessTimestamp(a, b time.Time) bool {
	return a.Compare(b) < 0
}

func LessDuration(a, b time.Duration) bool {
	return a < b
}

func LessEqualsInt64(a, b int64) bool {
	return a <= b
}

func LessEqualsInt64Uint64(a int64, b uint64) bool {
	return a < 0 || uint64(a) <= b
}

func LessEqualsInt64Double(a int64, b float64) bool {
	return float64(a) <= b
}

func LessEqualsUint64(a, b uint64) bool {
	return a <= b
}

func LessEqualsUint64Int64(a uint64, b int64) bool {
	return b >= 0 && a <= uint64(b)
}

func LessEqualsUint64Double(a uint64, b float64) bool {
	return float64(a) <= b
}

func LessEqualsDouble(a, b float64) bool {
	return a <= b
}

func LessEqualsDoubleInt64(a float64, b int64) bool {
	return a <= float64(b)
}

func LessEqualsDoubleUint64(a float64, b uint64) bool {
	return a <= float64(b)
}

func LessEqualsBool(a, b bool) bool {
	return !a || b
}

func LessEqualsString(a, b string) bool {
	return a <= b
}

func LessEqualsBytes(a, b []byte) bool {
	return bytes.Compare(a, b) <= 0
}

func LessEqualsTimestamp(a, b time.Time) bool {
	return a.Compare(b) <= 0
}

func LessEqualsDuration(a, b time.Duration) bool {
	return a <= b
}

func GreaterInt64(a, b int64) bool {
	return a > b
}

func GreaterInt64Uint64(a int64, b uint64) bool {
	return a >= 0 && uint64(a) > b
}

func GreaterInt64Double(a int64, b float64) bool {
	return float64(a) > b
}

func GreaterUint64(a, b uint64) bool {
	return a > b
}

func GreaterUint64Int64(a uint64, b int64) bool {
	return b < 0 || a > uint64(b)
}

func GreaterUint64Double(a uint64, b float64) bool {
	return float64(a) > b
}

func GreaterDouble(a, b float64) bool {
	return a > b
}

func GreaterDoubleInt64(a float64, b int64) bool {
	return a > float64(b)
}

func GreaterDoubleUint64(a float64, b uint64) bool {
	return a > float64(b)
}

func GreaterBool(a, b bool) bool {
	return a && !b
}

func GreaterString(a, b string) bool {
	return a > b
}

func GreaterBytes(a, b []byte) bool {
	return bytes.Compare(a, b) > 0
}

func GreaterTimestamp(a, b time.Time) bool {
	return a.Compare(b) > 0
}

func GreaterDuration(a, b time.Duration) bool {
	return a > b
}

func GreaterEqualsInt64(a, b int64) bool {
	return a >= b
}

func GreaterEqualsInt64Uint64(a int64, b uint64) bool {
	return a >= 0 && uint64(a) >= b
}

func GreaterEqualsInt64Double(a int64, b float64) bool {
	return float64(a) >= b
}

func GreaterEqualsUint64(a, b uint64) bool {
	return a >= b
}

func GreaterEqualsUint64Int64(a uint64, b int64) bool {
	return b < 0 || a >= uint64(b)
}

func GreaterEqualsUint64Double(a uint64, b float64) bool {
	return float64(a) >= b
}

func GreaterEqualsDouble(a, b float64) bool {
	return a >= b
}

func GreaterEqualsDoubleInt64(a float64, b int64) bool {
	return a >= float64(b)
}

func GreaterEqualsDoubleUint64(a float64, b uint64) bool {
	return a >= float64(b)
}

func GreaterEqualsBool(a, b bool) bool {
	return a || !b
}

func GreaterEqualsString(a, b string) bool {
	return a >= b
}

func GreaterEqualsBytes(a, b []byte) bool {
	return bytes.Compare(a, b) >= 0
}

func GreaterEqualsTimestamp(a, b time.Time) bool {
	return a.Compare(b) >= 0
}

func GreaterEqualsDuration(a, b time.Duration) bool {
	return a >= b
}

func AddInt64(a, b int64) int64 {
	return a + b
}

func AddUint64(a, b uint64) uint64 {
	return a + b
}

func AddDouble(a, b float64) float64 {
	return a + b
}

func AddString(a, b string) string {
	return a + b
}

func AddBytes(a, b []byte) []byte {
	return append(a, b...)
}

func AddList[T any](a, b []T) []T {
	return append(a, b...)
}

func AddTimestampDuration(a time.Time, b time.Duration) time.Time {
	return a.Add(b)
}

func AddDurationDuration(a time.Duration, b time.Duration) time.Duration {
	return a + b
}

func SubtractInt64(a, b int64) int64 {
	return a - b
}

func SubtractUint64(a, b uint64) uint64 {
	return a - b
}

func SubtractDouble(a, b float64) float64 {
	return a - b
}

func SubtractTimestampTimestamp(a time.Time, b time.Time) time.Duration {
	return a.Sub(b)
}

func SubtractTimestampDuration(a time.Time, b time.Duration) time.Time {
	return a.Add(-b)
}

func SubtractDurationDuration(a time.Duration, b time.Duration) time.Duration {
	return a - b
}

func MultiplyInt64(a, b int64) int64 {
	return a * b
}

func MultiplyUint64(a, b uint64) uint64 {
	return a * b
}

func MultiplyDouble(a, b float64) float64 {
	return a * b
}

func DivideInt64(a, b int64) (int64, error) {
	if b == 0 {
		return 0, errors.New("divide by 0")
	}
	return a / b, nil
}

func DivideUint64(a, b uint64) (uint64, error) {
	if b == 0 {
		return 0, errors.New("divide by 0")
	}
	return a / b, nil
}

func DivideDouble(a, b float64) float64 {
	return a / b
}

func ModuloInt64(a, b int64) (int64, error) {
	if b == 0 {
		return 0, errors.New("divide by 0")
	}
	return a % b, nil
}

func ModuloUint64(a, b uint64) (uint64, error) {
	if b == 0 {
		return 0, errors.New("divide by 0")
	}
	return a % b, nil
}

func NegateInt64(a int64) int64 {
	return -a
}

func NegateDouble(a float64) float64 {
	return -a
}

func IndexList[T any](a []T, b int64) (T, error) {
	// Check bounds.
	if b < 0 || b >= int64(len(a)) {
		var zeroT T
		return zeroT, fmt.Errorf("index %d out of range", b)
	}

	return a[b], nil
}

func IndexMap[K comparable, V any](a map[K]V, b K) (V, error) {
	elem, ok := a[b]
	if ok {
		return elem, nil
	}

	// Handle numeric equality for int and uint types as map keys.
	switch b := any(b).(type) {
	case int64:
		if b >= 0 {
			switch a := any(a).(type) {
			case map[any]V:
				if elem, ok := a[uint64(b)]; ok {
					return elem, nil
				}
			}
		}
	case uint64:
		if b <= math.MaxInt64 {
			switch a := any(a).(type) {
			case map[any]V:
				if elem, ok := a[int64(b)]; ok {
					return elem, nil
				}
			}
		}
	}

	var zeroV V
	return zeroV, fmt.Errorf("no such key %v", b)
}

func InList[T any, U any](a T, b []T) bool {
	for _, elem := range b {
		if Equals(elem, a) {
			return true
		}
	}
	return false
}

func InMap[K comparable, V any](a K, b map[K]V) bool {
	if _, ok := b[a]; ok {
		return true
	}

	// Handle numeric equality for int and uint types as map keys.
	switch a := any(a).(type) {
	case int64:
		if a >= 0 {
			switch b := any(b).(type) {
			case map[any]V:
				if _, ok := b[uint64(a)]; ok {
					return true
				}
			}
		}
	case uint64:
		if a <= math.MaxInt64 {
			switch b := any(b).(type) {
			case map[any]V:
				if _, ok := b[int64(a)]; ok {
					return true
				}
			}
		}
	}

	return false
}
