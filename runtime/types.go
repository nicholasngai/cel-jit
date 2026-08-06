package runtime

import (
	"fmt"
	"iter"
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
