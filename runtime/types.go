package runtime

import (
	"fmt"
	"reflect"
)

func ToListValue[T any](v any) ([]T, error) {
	switch v := any(v).(type) {
	case []T:
		return v, nil
	default:
		listVal, err := toSlice(reflect.ValueOf(v), reflect.TypeFor[T]())
		if err != nil {
			return nil, err
		}
		return listVal.Interface().([]T), nil
	}
}

func ToMapValue[K comparable, V any](v any) (map[K]V, error) {
	switch v := any(v).(type) {
	case map[K]V:
		return v, nil
	default:
		mapVal, err := toMap(reflect.ValueOf(v), reflect.TypeFor[K](), reflect.TypeFor[V]())
		if err != nil {
			return nil, err
		}
		return mapVal.Interface().(map[K]V), nil
	}
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
