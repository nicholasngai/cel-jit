package runtime

import (
	"reflect"
	"testing"
)

func TestToSlice(t *testing.T) {
	s, err := toSlice(reflect.ValueOf([]any{"foo", "bar"}), reflect.TypeFor[string]())
	if err != nil {
		t.Errorf("Failed to convert []any to []string: %v", err)
		return
	}
	if !reflect.DeepEqual([]string{"foo", "bar"}, s.Interface()) {
		t.Errorf("Converted value is %v, not [foo bar]", s.Interface())
		return
	}
}

func TestToSliceNested(t *testing.T) {
	s, err := toSlice(reflect.ValueOf([]any{[]any{"foo", "bar"}, []string{"fizz", "buzz"}}), reflect.TypeFor[[]string]())
	if err != nil {
		t.Errorf("Failed to convert []any to [][]string: %v", err)
		return
	}
	if !reflect.DeepEqual([][]string{{"foo", "bar"}, {"fizz", "buzz"}}, s.Interface()) {
		t.Errorf("Converted value is %v, not [[foo bar] [fizz buzz]]", s.Interface())
		return
	}
}

func TestToMap(t *testing.T) {
	s, err := toMap(reflect.ValueOf(map[any]any{"foo": 1, "bar": 2}), reflect.TypeFor[string](), reflect.TypeFor[int]())
	if err != nil {
		t.Errorf("Failed to convert map[any]any to map[string]int: %v", err)
		return
	}
	if !reflect.DeepEqual(map[string]int{"foo": 1, "bar": 2}, s.Interface()) {
		t.Errorf("Converted value is %v, not [foo:1 bar:2]", s.Interface())
		return
	}
}

func TestToMapNested(t *testing.T) {
	s, err := toMap(reflect.ValueOf(map[any]any{"a": map[any]any{"foo": 1, "bar": 2}, "b": map[string]any{"fizz": 3}, "c": map[any]int{"buzz": 4}}), reflect.TypeFor[string](), reflect.TypeFor[map[string]int]())
	if err != nil {
		t.Errorf("Failed to convert map[any]any to map[string]map[string]int: %v", err)
		return
	}
	if !reflect.DeepEqual(map[string]map[string]int{"a": {"foo": 1, "bar": 2}, "b": {"fizz": 3}, "c": {"buzz": 4}}, s.Interface()) {
		t.Errorf("Converted value is %v, not [a:[foo:1 bar:2] b:[fizz:3] c:[buzz:4]]", s.Interface())
		return
	}
}

func TestToSliceAndMapNested(t *testing.T) {
	s, err := toSlice(reflect.ValueOf([]any{map[any]any{"foo": []any{1, 2}}, map[string]any{"bar": []any{3, 4}}, map[any][]int{"fizzbuzz": {5, 6}}}), reflect.TypeFor[map[string][]int]())
	if err != nil {
		t.Errorf("Failed to convert map[any]any to []map[string][]int: %v", err)
		return
	}
	if !reflect.DeepEqual([]map[string][]int{{"foo": {1, 2}}, {"bar": {3, 4}}, {"fizzbuzz": {5, 6}}}, s.Interface()) {
		t.Errorf("Converted value is %v, not [foo:[1 2] bar:[3 4] fizzbuzz:[5 6]]", s.Interface())
		return
	}
}
