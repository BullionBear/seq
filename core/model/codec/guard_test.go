package codec_test

import (
	"reflect"
	"testing"
)

// TestWireTypesArePOD walks every registered wire type recursively and fails
// on any field whose kind cannot be safely memcpy'd: pointers, slices, maps,
// strings, chans, funcs, interfaces, and unsafe pointers. Adding such a field
// to a wire type silently corrupts the wire format; this test makes it a CI
// failure instead.
func TestWireTypesArePOD(t *testing.T) {
	for _, wt := range wireTypes {
		t.Run(wt.name, func(t *testing.T) {
			assertPOD(t, reflect.TypeOf(wt.zero), wt.name)
		})
	}
}

func assertPOD(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		return
	case reflect.Array:
		assertPOD(t, typ.Elem(), path+"[...]")
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			assertPOD(t, f.Type, path+"."+f.Name)
		}
	default:
		t.Errorf("%s has non-POD kind %s: wire types must not contain pointers, slices, maps, strings, chans, funcs, or interfaces", path, typ.Kind())
	}
}

// TestWireTypeLayoutGolden asserts the size and every field offset of every
// wire type against the golden constants in registry_test.go. The memcpy wire
// format is exactly the compiler's struct layout, so any change here (field
// added, removed, reordered, or retyped) invalidates historical msglogs.
// A failure means: either revert the layout change, or bump the msglog schema
// version (core/msgbus) and regenerate the golden constants deliberately.
func TestWireTypeLayoutGolden(t *testing.T) {
	for _, wt := range wireTypes {
		t.Run(wt.name, func(t *testing.T) {
			typ := reflect.TypeOf(wt.zero)
			if typ.Size() != wt.size {
				t.Errorf("sizeof(%s) = %d, golden = %d", wt.name, typ.Size(), wt.size)
			}
			if typ.NumField() != len(wt.fields) {
				t.Fatalf("%s has %d fields, golden has %d", wt.name, typ.NumField(), len(wt.fields))
			}
			for i, g := range wt.fields {
				f := typ.Field(i)
				if f.Name != g.name {
					t.Errorf("%s field %d is %q, golden = %q", wt.name, i, f.Name, g.name)
				}
				if f.Offset != g.offset {
					t.Errorf("offsetof(%s.%s) = %d, golden = %d", wt.name, f.Name, f.Offset, g.offset)
				}
			}
		})
	}
}
