package common

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/openapi"
)

// ErrNotFound is wrapped by the errors the provider makes itself when a lookup
// finds no object, such as a list of role assignments without the one it reads.
var ErrNotFound = errors.New("not found")

// APIStatusError is a failed response to a request the provider sends itself with
// net/http instead of through the SDK.
type APIStatusError struct {
	StatusCode int
	Body       string
}

func (e *APIStatusError) Error() string {
	return fmt.Sprintf("API request failed with status %d: %s", e.StatusCode, e.Body)
}

// IsNotFoundErr reports whether err says the Permit API has no such object. It
// decides from the error's type and status code, never from its message, so an
// error that only mentions "not found", such as a missing API token, does not count.
// It recognises:
//   - the SDK's PermitError with the NotFound error code, which the SDK returns
//     for a 404 on most calls;
//   - the SDK's GenericOpenAPIError for a 404, which calls such as
//     ResourceAttributes.Get return unwrapped, with the response's status line as
//     the message and no status code field;
//   - an APIStatusError with status 404;
//   - an error that wraps ErrNotFound.
func IsNotFoundErr(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var permitErr permitErrors.PermitError
	if errors.As(err, &permitErr) {
		return permitErr.ErrorCode == permitErrors.NotFound
	}
	var statusErr *APIStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusNotFound
	}
	var openAPIErr *openapi.GenericOpenAPIError
	if errors.As(err, &openAPIErr) {
		return statusLineCode(openAPIErr.Error()) == http.StatusNotFound
	}
	return false
}

// statusLineCode returns the status code at the start of an HTTP status line such
// as "404 Not Found", or 0 when the line does not start with one.
func statusLineCode(statusLine string) int {
	code, _, _ := strings.Cut(statusLine, " ")
	number, err := strconv.Atoi(code)
	if err != nil {
		return 0
	}
	return number
}
