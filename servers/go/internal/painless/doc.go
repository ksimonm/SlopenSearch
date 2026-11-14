// Package painless is an interpreter for the Painless scripting language of
// OpenSearch: lexer, parser, static type checks, interpreter, runtime values
// with Java semantics, the class library (incl. java.time and regex) and the
// script contexts with their compilation cache.
//
// It has no server state: documents are reached through DocMap callbacks and
// extra functions through Bindings.
package painless

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
