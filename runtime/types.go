package runtime

import (
	"fmt"
	"iter"
	"reflect"
	"time"
)

// DynValue represents a dynamically typed runtime value.
type DynValue struct {
	v   any
	err error
}

func (v DynValue) Val() any {
	return v.v
}

func (v DynValue) Err() error {
	return v.err
}

func (v DynValue) DynValue() DynValue {
	return v
}

func (v DynValue) IntValue() IntValue {
	if v.err != nil {
		return IntValue{err: v.err}
	}

	intVal, ok := v.v.(int64)
	if !ok {
		return IntValue{err: fmt.Errorf("%v is not an int", v.v)}
	}

	return IntValueOf(intVal)
}

func (v DynValue) UintValue() UintValue {
	if v.err != nil {
		return UintValue{err: v.err}
	}

	uintVal, ok := v.v.(uint64)
	if !ok {
		return UintValue{err: fmt.Errorf("%v is not a uint", v.v)}
	}

	return UintValueOf(uintVal)
}

func (v DynValue) DoubleValue() DoubleValue {
	if v.err != nil {
		return DoubleValue{err: v.err}
	}

	doubleVal, ok := v.v.(float64)
	if !ok {
		return DoubleValue{err: fmt.Errorf("%v is not a double", v.v)}
	}

	return DoubleValueOf(doubleVal)
}

func (v DynValue) BoolValue() BoolValue {
	if v.err != nil {
		return BoolValue{err: v.err}
	}

	boolVal, ok := v.v.(bool)
	if !ok {
		return BoolValue{err: fmt.Errorf("%v is not a bool", v.v)}
	}

	return BoolValueOf(boolVal)
}

func (v DynValue) StringValue() StringValue {
	if v.err != nil {
		return StringValue{err: v.err}
	}

	stringVal, ok := v.v.(string)
	if !ok {
		return StringValue{err: fmt.Errorf("%v is not a string", v.v)}
	}

	return StringValueOf(stringVal)
}

func (v DynValue) BytesValue() BytesValue {
	if v.err != nil {
		return BytesValue{err: v.err}
	}

	bytesVal, ok := v.v.([]byte)
	if !ok {
		return BytesValue{err: fmt.Errorf("%v is not a bytes", v.v)}
	}

	return BytesValueOf(bytesVal)
}

func (v DynValue) TimestampValue() TimestampValue {
	if v.err != nil {
		return TimestampValue{err: v.err}
	}

	timestampVal, ok := v.v.(time.Time)
	if !ok {
		return TimestampValue{err: fmt.Errorf("%v is not a timestamp", v.v)}
	}

	return TimestampValueOf(timestampVal)
}

func (v DynValue) DurationValue() DurationValue {
	if v.err != nil {
		return DurationValue{err: v.err}
	}

	durationVal, ok := v.v.(time.Duration)
	if !ok {
		return DurationValue{err: fmt.Errorf("%v is not a duration", v.v)}
	}

	return DurationValueOf(durationVal)
}

func (v DynValue) NullValue() NullValue {
	if v.err != nil {
		return NullValue{err: v.err}
	}

	if v.v != struct{}{} {
		return NullValue{err: fmt.Errorf("%v is not null", v.v)}
	}

	return NullValue{}
}

func ToListValue[T any](v DynValue) ListValue[T] {
	if v.err != nil {
		return ListValue[T]{err: v.err}
	}

	if vList, ok := v.v.([]T); ok {
		return ListValue[T]{v: vList}
	}

	listVal, err := toSlice(reflect.ValueOf(v.v), reflect.TypeFor[T]())
	if err != nil {
		return ListValue[T]{err: err}
	}

	return ListValueOf(listVal.Interface().([]T))
}

func toSlice(v reflect.Value, elemType reflect.Type) (reflect.Value, error) {
	if v.Type() == reflect.SliceOf(elemType) {
		return v, nil
	}

	if v.Kind() != reflect.Slice {
		return reflect.Value{}, fmt.Errorf("%v is not a list", v)
	}

	result := reflect.MakeSlice(reflect.SliceOf(elemType), v.Len(), v.Len())
	for i := range v.Len() {
		elem := v.Index(i)
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}
		if !elem.Type().AssignableTo(elemType) {
			if elemType.Kind() == reflect.Slice {
				sliceElem, err := toSlice(elem, elemType.Elem())
				if err != nil {
					return reflect.Value{}, fmt.Errorf("element %d: %w", i, err)
				}
				elem = sliceElem
			} else if elemType.Kind() == reflect.Map {
				mapElem, err := toMap(elem, elemType.Key(), elemType.Elem())
				if err != nil {
					return reflect.Value{}, fmt.Errorf("element %v: %w", i, err)
				}
				elem = mapElem
			} else {
				return reflect.Value{}, fmt.Errorf("element %d: %v is not a %v", i, elem, elemType)
			}
		}
		result.Index(i).Set(elem)
	}

	return result, nil
}

func toMap(v reflect.Value, keyType reflect.Type, valueType reflect.Type) (reflect.Value, error) {
	if v.Type() == reflect.MapOf(keyType, valueType) {
		return v, nil
	}

	if v.Kind() != reflect.Map {
		return reflect.Value{}, fmt.Errorf("%v is not a map", v)
	}

	result := reflect.MakeMap(reflect.MapOf(keyType, valueType))
	mapIter := v.MapRange()
	for mapIter.Next() {
		key := mapIter.Key()
		if key.Kind() == reflect.Interface {
			key = key.Elem()
		}
		if !key.Type().AssignableTo(keyType) {
			return reflect.Value{}, fmt.Errorf("key %v is not a %v", key, keyType)
		}
		value := mapIter.Value()
		if value.Kind() == reflect.Interface {
			value = value.Elem()
		}
		if !value.Type().AssignableTo(valueType) {
			if valueType.Kind() == reflect.Slice {
				sliceValue, err := toSlice(value, valueType.Elem())
				if err != nil {
					return reflect.Value{}, fmt.Errorf("key %v: %w", key, err)
				}
				value = sliceValue
			} else if valueType.Kind() == reflect.Map {
				mapValue, err := toMap(value, valueType.Key(), valueType.Elem())
				if err != nil {
					return reflect.Value{}, fmt.Errorf("key %v: %w", key, err)
				}
				value = mapValue
			} else {
				return reflect.Value{}, fmt.Errorf("key %v: %v is not a %v", key, value, valueType)
			}
		}
		result.SetMapIndex(key, value)
	}

	return result, nil
}

// DynValueOf returns a [DynValue] for the given value.
func DynValueOf(v any) DynValue {
	return DynValue{v: v}
}

// DynValueOfSlice returns a [DynValue] containing a slice from an iterator of
// [DynValue]s. If any value is an error, it returns the first error value
// instead.
func DynValueOfSlice(elems iter.Seq[DynValue], len int) DynValue {
	listVal := make([]any, 0, len)
	for elem := range elems {
		if elem.err != nil {
			return elem
		}
		listVal = append(listVal, elem.v)
	}
	return DynValue{v: listVal}
}

// DynValueOfMap returns a [DynValue] containing a map from an iterator of
// key-value [DynValue] pairs. If any value is an error, it returns the first
// error value instead.
func DynValueOfMap(entries iter.Seq2[DynValue, DynValue], len int) DynValue {
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
	return DynValue{v: mapVal}
}

func DynErrorOf(err error) DynValue {
	return DynValue{err: err}
}

// IntValue represents a statically typed int value.
type IntValue struct {
	v   int64
	err error
}

func (v IntValue) Val() int64 {
	return v.v
}

func (v IntValue) Err() error {
	return v.err
}

func (v IntValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v IntValue) IntValue() IntValue {
	return v
}

func IntValueOf(v int64) IntValue {
	return IntValue{v: v}
}

// UintValue represents a statically typed uint value.
type UintValue struct {
	v   uint64
	err error
}

func (v UintValue) Val() uint64 {
	return v.v
}

func (v UintValue) Err() error {
	return v.err
}

func (v UintValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v UintValue) UintValue() UintValue {
	return v
}

func UintValueOf(v uint64) UintValue {
	return UintValue{v: v}
}

// DoubleValue represents a statically typed double value.
type DoubleValue struct {
	v   float64
	err error
}

func (v DoubleValue) Val() float64 {
	return v.v
}

func (v DoubleValue) Err() error {
	return v.err
}

func (v DoubleValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v DoubleValue) DoubleValue() DoubleValue {
	return v
}

func DoubleValueOf(v float64) DoubleValue {
	return DoubleValue{v: v}
}

// BoolValue represents a statically typed bool value.
type BoolValue struct {
	v   bool
	err error
}

func (v BoolValue) Val() bool {
	return v.v
}

func (v BoolValue) Err() error {
	return v.err
}

func (v BoolValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v BoolValue) BoolValue() BoolValue {
	return v
}

func BoolValueOf(v bool) BoolValue {
	return BoolValue{v: v}
}

// StringValue represents a statically typed string value.
type StringValue struct {
	v   string
	err error
}

func (v StringValue) Val() string {
	return v.v
}

func (v StringValue) Err() error {
	return v.err
}

func (v StringValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v StringValue) StringValue() StringValue {
	return v
}

func StringValueOf(v string) StringValue {
	return StringValue{v: v}
}

// BytesValue represents a statically typed bytes value.
type BytesValue struct {
	v   []byte
	err error
}

func (v BytesValue) Val() []byte {
	return v.v
}

func (v BytesValue) Err() error {
	return v.err
}

func (v BytesValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v BytesValue) BytesValue() BytesValue {
	return v
}

func BytesValueOf(v []byte) BytesValue {
	return BytesValue{v: v}
}

// TimestampValue represents a statically typed timestamp value.
type TimestampValue struct {
	v   time.Time
	err error
}

func (v TimestampValue) Val() time.Time {
	return v.v
}

func (v TimestampValue) Err() error {
	return v.err
}

func (v TimestampValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v TimestampValue) TimestampValue() TimestampValue {
	return v
}

func TimestampValueOf(v time.Time) TimestampValue {
	return TimestampValue{v: v}
}

// DurationValue represents a statically typed duration value.
type DurationValue struct {
	v   time.Duration
	err error
}

func (v DurationValue) Val() time.Duration {
	return v.v
}

func (v DurationValue) Err() error {
	return v.err
}

func (v DurationValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(v.v)
}

func (v DurationValue) DurationValue() DurationValue {
	return v
}

func DurationValueOf(v time.Duration) DurationValue {
	return DurationValue{v: v}
}

// NullValue represents a statically typed null value.
type NullValue struct {
	err error
}

func (NullValue) Val() struct{} {
	return struct{}{}
}

func (v NullValue) Err() error {
	return v.err
}

func (v NullValue) DynValue() DynValue {
	if v.err != nil {
		return DynErrorOf(v.err)
	}
	return DynValueOf(struct{}{})
}

func (v NullValue) NullValue() NullValue {
	return v
}

// ListValue represents a statically typed list value.
type ListValue[T any] struct {
	v   []T
	err error
}

func (v ListValue[T]) Val() []T {
	return v.v
}

func (v ListValue[T]) Err() error {
	return v.err
}

func (v ListValue[T]) DynValue() DynValue {
	if v.err != nil {
		return DynValue{err: v.err}
	}
	return DynValue{v: v.v}
}

func ListValueOf[T any](v []T) ListValue[T] {
	return ListValue[T]{v: v}
}
