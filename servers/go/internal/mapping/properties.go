package mapping

// EnsureProperties returns the properties of a mapping, adding an empty one.
func EnsureProperties(mappings map[string]any) map[string]any {
	props, ok := mappings["properties"].(map[string]any)
	if !ok {
		props = map[string]any{}
		mappings["properties"] = props
	}
	return props
}
