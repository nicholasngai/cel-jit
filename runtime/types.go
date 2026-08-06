package runtime

import (
	"fmt"
	"reflect"
	"time"
)

// DynValue represents a dynamically typed runtime value.
type DynValue struct {
	Val any
	Err error
}

func (v DynValue) DynValue() DynValue {
	return v
}

func (v DynValue) IntValue() IntValue {
	if v.Err != nil {
		return IntValue{Err: v.Err}
	}

	intVal, ok := v.Val.(int64)
	if !ok {
		return IntValue{Err: fmt.Errorf("%v is not an int", v.Val)}
	}

	return IntValue{Val: intVal}
}

func (v DynValue) UintValue() UintValue {
	if v.Err != nil {
		return UintValue{Err: v.Err}
	}

	uintVal, ok := v.Val.(uint64)
	if !ok {
		return UintValue{Err: fmt.Errorf("%v is not a uint", v.Val)}
	}

	return UintValue{Val: uintVal}
}

func (v DynValue) DoubleValue() DoubleValue {
	if v.Err != nil {
		return DoubleValue{Err: v.Err}
	}

	doubleVal, ok := v.Val.(float64)
	if !ok {
		return DoubleValue{Err: fmt.Errorf("%v is not a double", v.Val)}
	}

	return DoubleValue{Val: doubleVal}
}

func (v DynValue) BoolValue() BoolValue {
	if v.Err != nil {
		return BoolValue{Err: v.Err}
	}

	boolVal, ok := v.Val.(bool)
	if !ok {
		return BoolValue{Err: fmt.Errorf("%v is not a bool", v.Val)}
	}

	return BoolValue{Val: boolVal}
}

func (v DynValue) StringValue() StringValue {
	if v.Err != nil {
		return StringValue{Err: v.Err}
	}

	stringVal, ok := v.Val.(string)
	if !ok {
		return StringValue{Err: fmt.Errorf("%v is not a string", v.Val)}
	}

	return StringValue{Val: stringVal}
}

func (v DynValue) BytesValue() BytesValue {
	if v.Err != nil {
		return BytesValue{Err: v.Err}
	}

	bytesVal, ok := v.Val.([]byte)
	if !ok {
		return BytesValue{Err: fmt.Errorf("%v is not a bytes", v.Val)}
	}

	return BytesValue{Val: bytesVal}
}

func (v DynValue) TimestampValue() TimestampValue {
	if v.Err != nil {
		return TimestampValue{Err: v.Err}
	}

	timestampVal, ok := v.Val.(time.Time)
	if !ok {
		return TimestampValue{Err: fmt.Errorf("%v is not a timestamp", v.Val)}
	}

	return TimestampValue{Val: timestampVal}
}

func (v DynValue) DurationValue() DurationValue {
	if v.Err != nil {
		return DurationValue{Err: v.Err}
	}

	durationVal, ok := v.Val.(time.Duration)
	if !ok {
		return DurationValue{Err: fmt.Errorf("%v is not a duration", v.Val)}
	}

	return DurationValue{Val: durationVal}
}

func (v DynValue) NullValue() NullValue {
	if v.Err != nil {
		return NullValue{Err: v.Err}
	}

	if v.Val != struct{}{} {
		return NullValue{Err: fmt.Errorf("%v is not null", v.Val)}
	}

	return NullValue{}
}

func ToListValue[T any](v DynValue) ListValue[T] {
	if v.Err != nil {
		return ListValue[T]{Err: v.Err}
	}

	if vList, ok := v.Val.([]T); ok {
		return ListValue[T]{Val: vList}
	}

	listVal, err := toSlice(reflect.ValueOf(v.Val), reflect.TypeFor[T]())
	if err != nil {
		return ListValue[T]{Err: err}
	}

	return ListValue[T]{Val: listVal.Interface().([]T)}
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

// IntValue represents a statically typed int value.
type IntValue struct {
	Val int64
	Err error
}

func (v IntValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v IntValue) IntValue() IntValue {
	return v
}

// UintValue represents a statically typed uint value.
type UintValue struct {
	Val uint64
	Err error
}

func (v UintValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v UintValue) UintValue() UintValue {
	return v
}

// DoubleValue represents a statically typed double value.
type DoubleValue struct {
	Val float64
	Err error
}

func (v DoubleValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v DoubleValue) DoubleValue() DoubleValue {
	return v
}

// BoolValue represents a statically typed bool value.
type BoolValue struct {
	Val bool
	Err error
}

func (v BoolValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v BoolValue) BoolValue() BoolValue {
	return v
}

// StringValue represents a statically typed string value.
type StringValue struct {
	Val string
	Err error
}

func (v StringValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v StringValue) StringValue() StringValue {
	return v
}

// BytesValue represents a statically typed bytes value.
type BytesValue struct {
	Val []byte
	Err error
}

func (v BytesValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v BytesValue) BytesValue() BytesValue {
	return v
}

// TimestampValue represents a statically typed timestamp value.
type TimestampValue struct {
	Val time.Time
	Err error
}

func (v TimestampValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v TimestampValue) TimestampValue() TimestampValue {
	return v
}

// DurationValue represents a statically typed duration value.
type DurationValue struct {
	Val time.Duration
	Err error
}

func (v DurationValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v DurationValue) DurationValue() DurationValue {
	return v
}

// NullValue represents a statically typed null value.
type NullValue struct {
	Val struct{}
	Err error
}

func (v NullValue) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}

func (v NullValue) NullValue() NullValue {
	return v
}

// ListValue represents a statically typed list value.
type ListValue[T any] struct {
	Val []T
	Err error
}

func (v ListValue[T]) DynValue() DynValue {
	if v.Err != nil {
		return DynValue{Err: v.Err}
	}
	return DynValue{Val: v.Val}
}
