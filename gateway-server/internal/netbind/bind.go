package netbind

import "strings"

// SameBindAddress returns true when two bind addresses should be treated as
// overlapping listeners. Wildcard binds conflict with any explicit bind on the
// same protocol family.
func SameBindAddress(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	return a == "0.0.0.0" || b == "0.0.0.0" || a == "::" || b == "::"
}
