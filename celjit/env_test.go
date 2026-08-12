package celjit

import (
	"bytes"
	"reflect"
	"testing"
)

type testType struct {
	Int        int
	Int8       int8
	Int16      int16
	Int32      int32
	Int64      int64
	Uint       uint
	Uint8      uint8
	Uint16     uint16
	Uint32     uint32
	Uint64     uint64
	Uintptr    uintptr
	Float32    float32
	Float64    float64
	Complex64  complex64
	Complex128 complex128
	Bool       bool
	String     string
	Interface  interface{ SomeIrrelevantMethod() }

	Pointer *int
	Array   [8]int
	Slice   []int
	Map     map[int]int
	Chan    chan int

	_ [4]byte

	NestedType nestedType

	embeddedType
}

type nestedType struct {
	NestedInt int
}

type embeddedType struct {
	EmbeddedInt int
}

func TestWriteType(t *testing.T) {
	b := new(bytes.Buffer)

	if err := writeType(b, reflect.TypeFor[testType](), make(map[reflect.Type]string), new(int)); err != nil {
		t.Fatalf("Failed to write types: %v", err)
	}

	expected := `
type Struct_nestedType_1 struct {
	NestedInt int
}

type Struct_embeddedType_2 struct {
	EmbeddedInt int
}

type Struct_testType_0 struct {
	Int int
	Int8 int8
	Int16 int16
	Int32 int32
	Int64 int64
	Uint uint
	Uint8 uint8
	Uint16 uint16
	Uint32 uint32
	Uint64 uint64
	Uintptr uintptr
	Float32 float32
	Float64 float64
	Complex64 complex64
	Complex128 complex128
	Bool bool
	String string
	Interface any
	Pointer *int
	Array [8]int
	Slice []int
	Map map[int]int
	Chan chan int
	_ [4]uint8
	NestedType Struct_nestedType_1
	Struct_embeddedType_2
}
`
	if b.String() != expected {
		t.Fatalf("Got incorrect output:\n%s", b.String())
	}
}
