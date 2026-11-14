package settings

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"slopensearch/internal/jsonv"
)

func ValueString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return jsonv.FormatNumber(t)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func ValueInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), t == float64(int(t))
	case json.Number:
		n, err := t.Int64()
		return int(n), err == nil
	case int64:
		return int(t), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
