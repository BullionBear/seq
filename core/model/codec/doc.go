// Package codec is the single source of layout truth for fixed-size wire
// types: a generic, bounds-checked Encode/Decode pair (raw memcpy of the
// struct, little-endian host layout) and a Cursor writer for variable-size
// encoders.
//
// # Wire type contract
//
// The memcpy format is only sound for POD structs with a frozen layout.
// Every type passed to Encode/Decode/Put must therefore:
//
//  1. Contain no pointer, slice, map, string, chan, func, or interface
//     field at any nesting depth (enforced by TestWireTypesArePOD).
//  2. Keep its size and every field offset equal to the golden constants in
//     guard_test.go. Any layout change fails CI until the constants are
//     bumped deliberately (enforced by TestWireTypeLayoutGolden).
//
// # Style rule for wire structs
//
// Declare layout, don't inherit it: order fields largest-first (8-byte
// fields, then 4-byte, then smaller) and spell out any padding explicitly
// as `_ [N]byte` so the byte image is what the source says, not what the
// compiler chose. New wire types must be added to the registry in
// guard_test.go before they are published on the bus.
package codec
