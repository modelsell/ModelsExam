package common

import "unsafe"

// GetPointer returns a pointer to a copy of v.
func GetPointer[T any](v T) *T { return &v }

// StringToByteSlice views s as bytes without copying. The result must not be modified.
func StringToByteSlice(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
