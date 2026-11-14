package analysis

import (
	"slopensearch/internal/jregex"

	"slopensearch/internal/apierr"
)

// jregexError converts a jregex error into the 400 analysis error OpenSearch reports.
func jregexError(e *jregex.Error) *apierr.Error {
	return RequestError(e.Type, e.Reason)
}

// javaRegexFlags parses the pipe separated flag names accepted by OpenSearch.
func javaRegexFlags(s string) (int, *apierr.Error) {
	f, err := jregex.ParseFlags(s)
	if err != nil {
		return 0, jregexError(err)
	}
	return f, nil
}

// compileJavaRegexAPI compiles and converts syntax errors to API errors.
func compileJavaRegexAPI(pattern string, flags int) (*jregex.Regexp, *apierr.Error) {
	re, err := jregex.Compile(pattern, flags)
	if err != nil {
		return nil, RequestError("pattern_syntax_exception", err.Message())
	}
	return re, nil
}
