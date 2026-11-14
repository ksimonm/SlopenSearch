// Package apierr has the error type of the OpenSearch REST API: an exception
// with a type, a reason, an HTTP status and optional causes. How an Error is
// rendered into a response body is up to the HTTP layer.
package apierr

// Error is an OpenSearch exception.
type Error struct {
	Type          string
	Reason        string
	Status        int
	Extra         any
	Fields        map[string]any
	CausedBy      *Error
	TopType       string
	TopReason     string
	RootCause     *Error
	ForceTop      bool
	Phase         string
	Clause        string
	ClauseEnd     bool
	NoCausedBy    bool
	SearchFailure string
	Shard         Response // pre-rendered response (search shard failure), written as is
	Suppressed    []*Error
}

// Response is a complete, pre-rendered error response.
type Response interface {
	StatusCode() int
	ResponseBody() any
}

func (e *Error) Error() string { return e.Type + ": " + e.Reason }

// BadRequest is a 400 illegal_argument_exception.
func BadRequest(reason string) *Error {
	return &Error{Type: "illegal_argument_exception", Reason: reason, Status: 400}
}

// Request is a 400 error of the given type whose root cause is the same
// exception (as the _analyze API and analysis settings report them).
func Request(etype, reason string) *Error {
	return &Error{Type: etype, Status: 400, Reason: reason,
		RootCause: &Error{Type: etype, Status: 400, Reason: reason}}
}
