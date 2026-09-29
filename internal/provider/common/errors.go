package common

import "strings"

// IsNotFoundErr reports whether err represents a "not found" response from the
// Permit API. The API returns this in a few textual forms depending on the
// endpoint (for example "404 Not Found"), so the match is case-insensitive.
func IsNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
