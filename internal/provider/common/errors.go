package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// maxMessageLength is the most bytes of an API response that APIErrorDetail shows.
const maxMessageLength = 1000

// apiKeyPattern matches a Permit API key, which APIErrorDetail never shows, even
// when a response repeats one.
var apiKeyPattern = regexp.MustCompile(`permit_key_[A-Za-z0-9_-]+`)

// APIErrorDetail returns the detail of the diagnostic for err, the error of an
// operation such as "create" on the objectType object with this key, for example:
//
//	Unable to create resource "docs": 409 Conflict: A resource with this key already exists
//
// When err is, or wraps, an error response of the Permit API, the detail gives the
// response's HTTP status and the message in its body, or the body itself when it
// has no message. The body is cut to maxMessageLength bytes and any API key in it is
// redacted. Request headers are never read. For any other error, including one
// about a successful response, such as a body the SDK cannot decode, the detail
// ends with err's text. An empty key is left out.
func APIErrorDetail(operation, objectType, key string, err error) string {
	object := objectType
	if key != "" {
		object = fmt.Sprintf("%s %q", objectType, key)
	}
	return fmt.Sprintf("Unable to %s %s: %s", operation, object, describeError(err))
}

// describeError returns err's text, with the status and body of the API response
// in place of the SDK's text when err has a failed response. A response with a
// status below 300 succeeded, and its body is the object itself, which may hold
// secrets such as a proxy config's credentials, so it is never shown.
func describeError(err error) string {
	if err == nil {
		return "unknown error"
	}
	status, body, apiErrText := findAPIResponse(err)
	if status < http.StatusMultipleChoices {
		return redact(err.Error())
	}
	// Keep the context that the provider's own wrapping put in front of the API error.
	prefix, wrapped := strings.CutSuffix(err.Error(), apiErrText)
	if !wrapped {
		prefix = ""
	}
	text := fmt.Sprintf("%s%d %s", prefix, status, http.StatusText(status))
	if message := apiMessage(body); message != "" {
		text += ": " + message
	}
	return redact(text)
}

// findAPIResponse returns the HTTP status and body of the API response in err's
// chain and the text of the error that holds them, or a zero status when err has
// no response.
func findAPIResponse(err error) (status int, body, apiErrText string) {
	var permitErr permitErrors.PermitError
	if errors.As(err, &permitErr) {
		return permitErr.StatusCode, permitErr.ResponseBody, permitErr.Error()
	}
	var statusErr *APIStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode, statusErr.Body, statusErr.Error()
	}
	var openAPIErr *openapi.GenericOpenAPIError
	if errors.As(err, &openAPIErr) {
		return statusLineCode(openAPIErr.Error()), string(openAPIErr.Body()), openAPIErr.Error()
	}
	return 0, "", ""
}

// apiMessage returns the message of an API error body: the "message" of the
// Permit API's error details, the "detail" of a validation error, or else the
// whole body.
func apiMessage(body string) string {
	body = strings.TrimSpace(body)
	var fields struct {
		Message string          `json:"message"`
		Detail  json.RawMessage `json:"detail"`
	}
	if json.Unmarshal([]byte(body), &fields) == nil {
		if fields.Message != "" {
			return shorten(fields.Message)
		}
		if detail := validationDetail(fields.Detail); detail != "" {
			return shorten(detail)
		}
	}
	return shorten(body)
}

// validationDetail returns the "detail" of a validation error body, which is a
// string or a list of problems, each with a field's location and a message. A list
// is returned as "location: message" pairs.
func validationDetail(detail json.RawMessage) string {
	var text string
	if json.Unmarshal(detail, &text) == nil {
		return text
	}
	var problems []struct {
		Loc []any  `json:"loc"`
		Msg string `json:"msg"`
	}
	if json.Unmarshal(detail, &problems) != nil {
		return ""
	}
	described := make([]string, 0, len(problems))
	for _, problem := range problems {
		location := make([]string, 0, len(problem.Loc))
		for _, part := range problem.Loc {
			location = append(location, fmt.Sprint(part))
		}
		described = append(described, strings.Join(location, ".")+": "+problem.Msg)
	}
	return strings.Join(described, "; ")
}

// shorten cuts text to maxMessageLength bytes without splitting a character.
func shorten(text string) string {
	if len(text) <= maxMessageLength {
		return text
	}
	cut := maxMessageLength
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "... (truncated)"
}

// redact replaces every Permit API key in text.
func redact(text string) string {
	return apiKeyPattern.ReplaceAllString(text, "[redacted API key]")
}

// AddReplaceOnlyUpdateError adds the error that Update reports for an objectType
// resource whose every attribute forces replacement. Terraform plans a
// replacement for any change to such a resource and never calls its Update, so
// reaching it is a provider bug.
func AddReplaceOnlyUpdateError(diags *diag.Diagnostics, objectType string) {
	diags.AddError(
		"Unable to update "+objectType,
		fmt.Sprintf("A %s cannot be updated in place; all attributes force replacement. "+
			"Please report this issue to the provider developers.", objectType),
	)
}
