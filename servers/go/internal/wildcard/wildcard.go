// Package wildcard matches simple "*" patterns (index names, filter_path
// segments, template patterns, ...).
package wildcard

import "strings"

// Match reports whether s matches pat, where "*" matches any run of
// characters.
func Match(pat, s string) bool {
	if pat == "*" || pat == s {
		return true
	}
	if !strings.Contains(pat, "*") {
		return false
	}
	parts := strings.Split(pat, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}
