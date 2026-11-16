// Package envname is the one rule for comparing environment variable names: exact where the system is case-sensitive,
// upper-cased where it is not (Windows).
package envname

import "strings"

// Key is name as it is compared: upper-cased when caseInsensitive.
func Key(name string, caseInsensitive bool) string {
	if caseInsensitive {
		return strings.ToUpper(name)
	}
	return name
}

// Equal reports whether a and b name the same variable.
func Equal(a, b string, caseInsensitive bool) bool {
	return a == b || caseInsensitive && strings.EqualFold(a, b)
}
