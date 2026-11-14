package apierr

// IndexNotFound is the 404 index_not_found_exception for index.
func IndexNotFound(index string) *Error {
	return &Error{Type: "index_not_found_exception", Reason: "no such index [" + index + "]", Status: 404}
}

// Parsing is a 400 parsing_exception.
func Parsing(reason string) *Error {
	return &Error{Type: "parsing_exception", Reason: reason, Status: 400}
}
