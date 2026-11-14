package mapping

import (
	"strconv"

	"slopensearch/internal/apierr"
)

func checkObjectDepth(v any, depth, limit int) *apierr.Error {
	if depth > limit {
		reason := "The depth of the field has exceeded the allowed limit of [" + strconv.Itoa(limit) + "]. This limit can be set by changing the [index.mapping.depth.limit] index level setting."
		return &apierr.Error{Type: "mapper_parsing_exception", Status: 400,
			Reason:    "failed to parse",
			RootCause: &apierr.Error{Type: "parse_exception", Reason: reason},
			CausedBy:  &apierr.Error{Type: "parse_exception", Reason: reason}}
	}
	switch t := v.(type) {
	case map[string]any:
		for _, e := range t {
			if err := checkObjectDepth(e, depth+1, limit); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range t {
			if err := checkObjectDepth(e, depth, limit); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkArrayDepth enforces index.mapping.depth.limit on arrays nested in
// arrays (DocumentParser's field array depth).
func checkArrayDepth(v any, depth, limit int) *apierr.Error {
	switch t := v.(type) {
	case map[string]any:
		for _, e := range t {
			if err := checkArrayDepth(e, depth, limit); err != nil {
				return err
			}
		}
	case []any:
		if depth+1 > limit {
			reason := "The depth of the nested array field has exceeded the allowed limit of [" + strconv.Itoa(limit) + "]. This limit can be set by changing the [index.mapping.depth.limit] index level setting."
			return &apierr.Error{Type: "mapper_parsing_exception", Status: 400,
				Reason:    "failed to parse",
				RootCause: &apierr.Error{Type: "parse_exception", Reason: reason},
				CausedBy:  &apierr.Error{Type: "parse_exception", Reason: reason}}
		}
		for _, e := range t {
			if err := checkArrayDepth(e, depth+1, limit); err != nil {
				return err
			}
		}
	}
	return nil
}
