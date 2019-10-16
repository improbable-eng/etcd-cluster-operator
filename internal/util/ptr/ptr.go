package ptr

// Int32 creates an int32 pointer
func Int32(i int32) *int32 {
	return &i
}

// Int64 creates an int64 pointer
func Int64(i int64) *int64 {
	return &i
}

// Bool creates a bool pointer
func Bool(b bool) *bool {
	return &b
}

// String creates a string pointer
func String(s string) *string {
	return &s
}

// StringValueOrNil return the referenced value or "<nil>" to mimic fmt.Printf("%+v", pointer)
func StringValueOrNil(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
