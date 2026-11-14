package mapping

func IsNumericType(t string) bool {
	switch t {
	case "integer", "long", "short", "byte", "float", "double", "half_float", "scaled_float", "date":
		return true
	}
	return false
}
