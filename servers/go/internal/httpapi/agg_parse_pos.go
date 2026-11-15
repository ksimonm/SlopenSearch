package httpapi

import (
	"fmt"
	"regexp"

	"slopensearch/internal/apierr"
)

var aggFieldParseErr = regexp.MustCompile(`^\[[a-z_]+\] failed to parse field \[([a-z_]+)\]$`)

// positionAggParseError puts the [line:col] of the field's value in front of
// an aggregation parser error ("[date_histogram] failed to parse field
// [time_zone]"), as the x-content parser reports it.
func positionAggParseError(err *apierr.Error, raw []byte) {
	if err == nil || err.Type != "x_content_parse_exception" {
		return
	}
	m := aggFieldParseErr.FindStringSubmatch(err.Reason)
	if m == nil {
		return
	}
	line, col := jsonKeyValuePos(raw, m[1])
	err.Reason = fmt.Sprintf("[%d:%d] %s", line, col, err.Reason)
}
