package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/openapi"
)

// TestIsNotFoundErr checks that IsNotFoundErr decides from the error's type and
// status code. Several errors that are not a 404 say "not found" in their message,
// so a helper that matches the message fails this test.
func TestIsNotFoundErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"SDK 404", sdkError(t, http.StatusNotFound), true},
		{"SDK 404, wrapped", fmt.Errorf("x: %w", sdkError(t, http.StatusNotFound)), true},
		{"SDK 403", sdkError(t, http.StatusForbidden), false},
		{"SDK 422", sdkError(t, http.StatusUnprocessableEntity), false},
		{"SDK 500", sdkError(t, http.StatusInternalServerError), false},
		{
			"SDK non-404 code with a not found message",
			permitErrors.NewPermitUnprocessableEntityError(errors.New("parent not found"), nil),
			false,
		},
		{"OpenAPI 404", openAPIError(t, http.StatusNotFound), true},
		{"OpenAPI 404, wrapped", fmt.Errorf("x: %w", openAPIError(t, http.StatusNotFound)), true},
		{"OpenAPI 400", openAPIError(t, http.StatusBadRequest), false},
		{"OpenAPI 500", openAPIError(t, http.StatusInternalServerError), false},
		{"HTTP 404", &APIStatusError{StatusCode: http.StatusNotFound, Body: "{}"}, true},
		{
			"HTTP 404, wrapped",
			fmt.Errorf("list roles: %w", &APIStatusError{StatusCode: http.StatusNotFound}),
			true,
		},
		{
			"HTTP 500 with a not found body",
			&APIStatusError{StatusCode: http.StatusInternalServerError, Body: "group not found"},
			false,
		},
		{"lookup miss", fmt.Errorf("role assignment %w", ErrNotFound), true},
		{"message only", errors.New("API token not found - configure api_key"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFoundErr(tt.err); got != tt.want {
				t.Errorf("IsNotFoundErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestNotFoundErrorMessages checks the messages the provider shows for the errors
// it makes itself.
func TestNotFoundErrorMessages(t *testing.T) {
	if got, want := ErrNotFound.Error(), "not found"; got != want {
		t.Errorf("ErrNotFound = %q, want %q", got, want)
	}
	statusErr := &APIStatusError{StatusCode: http.StatusConflict, Body: `{"detail":"exists"}`}
	if got, want := statusErr.Error(),
		`API request failed with status 409: {"detail":"exists"}`; got != want {
		t.Errorf("APIStatusError = %q, want %q", got, want)
	}
}

// TestAPIErrorDetail checks that a diagnostic names the operation, the object, the
// HTTP status and the API's message, keeps the provider's own context, and never
// shows an API key.
func TestAPIErrorDetail(t *testing.T) {
	const prefix = `Unable to create resource "docs": `
	// The cut at maxMessageLength falls inside a two-byte character.
	longBody := "a" + strings.Repeat("é", maxMessageLength)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			"SDK 409 with the API's error details",
			sdkErrorWithBody(t, http.StatusConflict,
				`{"error_code":"DUPLICATE_ENTITY","title":"Conflict",`+
					`"message":"A resource with this key already exists"}`),
			prefix + "409 Conflict: A resource with this key already exists",
		},
		{
			"SDK 400",
			sdkErrorWithBody(t, http.StatusBadRequest, `{"message":"Invalid condition"}`),
			prefix + "400 Bad Request: Invalid condition",
		},
		{
			"SDK 422 validation problems",
			sdkErrorWithBody(t, http.StatusUnprocessableEntity,
				`{"detail":[{"loc":["body","key"],"msg":"field required","type":"missing"},`+
					`{"loc":["body","actions",0],"msg":"not a dict","type":"type_error"}]}`),
			prefix + "422 Unprocessable Entity: body.key: field required; " +
				"body.actions.0: not a dict",
		},
		{
			"SDK 403 with a detail string",
			sdkErrorWithBody(t, http.StatusForbidden, `{"detail":"Environment is locked"}`),
			prefix + "403 Forbidden: Environment is locked",
		},
		{
			"SDK 429 with no body",
			sdkErrorWithBody(t, http.StatusTooManyRequests, ""),
			prefix + "429 Too Many Requests",
		},
		{
			"SDK 500 with a plain-text body",
			sdkErrorWithBody(t, http.StatusInternalServerError, "upstream timed out\n"),
			prefix + "500 Internal Server Error: upstream timed out",
		},
		{
			"SDK error details with an empty message",
			sdkErrorWithBody(t, http.StatusConflict,
				`{"error_code":"DUPLICATE_ENTITY","message":""}`),
			prefix + `409 Conflict: {"error_code":"DUPLICATE_ENTITY","message":""}`,
		},
		{
			"SDK error wrapped with the provider's context",
			fmt.Errorf("getting target role docs/viewer: %w",
				sdkErrorWithBody(t, http.StatusNotFound, `{"message":"Role not found"}`)),
			prefix + "getting target role docs/viewer: 404 Not Found: Role not found",
		},
		{
			"OpenAPI 404",
			openAPIError(t, http.StatusNotFound),
			prefix + "404 Not Found: object not found",
		},
		{
			"HTTP 409 from the provider's own request",
			fmt.Errorf("assign: %w", &APIStatusError{
				StatusCode: http.StatusConflict, Body: `{"message":"Already assigned"}`,
			}),
			prefix + "assign: 409 Conflict: Already assigned",
		},
		{
			"SDK connection error without a response",
			permitErrors.NewPermitConnectionError(errors.New("dial tcp: connection refused")),
			prefix + "ErrorCode: ConnectionError, ErrorType: general_error, " +
				"Message: dial tcp: connection refused",
		},
		{
			"provider error",
			fmt.Errorf("role assignment %w", ErrNotFound),
			prefix + "role assignment not found",
		},
		{
			"API key in the body",
			sdkErrorWithBody(t, http.StatusUnauthorized,
				`{"message":"Bad key permit_key_AbC123-x_y for this environment"}`),
			prefix + "401 Unauthorized: Bad key [redacted API key] for this environment",
		},
		{
			"long body",
			sdkErrorWithBody(t, http.StatusBadRequest, longBody),
			prefix + "400 Bad Request: " + longBody[:maxMessageLength-1] + "... (truncated)",
		},
		{"nil error", nil, prefix + "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := APIErrorDetail("create", "resource", "docs", tt.err); got != tt.want {
				t.Errorf("APIErrorDetail(%v) =\n%q, want\n%q", tt.err, got, tt.want)
			}
		})
	}

	got := APIErrorDetail("read", "role assignment", "", errors.New("boom"))
	if want := "Unable to read role assignment: boom"; got != want {
		t.Errorf("APIErrorDetail without a key = %q, want %q", got, want)
	}
}

// TestAPIErrorDetailHidesSuccessfulResponses answers a request with a 200 whose
// body the SDK cannot decode and holds a secret, and checks that the detail gives
// the decode error, not the status and the body, both for the generated client's
// error and for the SDK's error that wraps it with the response.
func TestAPIErrorDetailHidesSuccessfulResponses(t *testing.T) {
	const secret = "Bearer s3cr3t-t0ken"
	response, openAPIErr := openAPIResponse(t, http.StatusOK,
		`{"key": 5, "secret": "`+secret+`"}`)
	sdkErr := permitErrors.HttpErrorHandle(openAPIErr, response)
	var permitErr permitErrors.PermitError
	if !errors.As(sdkErr, &permitErr) || permitErr.StatusCode != http.StatusOK ||
		!strings.Contains(permitErr.ResponseBody, secret) {
		t.Fatalf("the SDK error %#v does not hold the 200 response and its body", sdkErr)
	}

	for name, err := range map[string]error{"OpenAPI": openAPIErr, "SDK": sdkErr} {
		t.Run(name, func(t *testing.T) {
			got := APIErrorDetail("read", "proxy config", "billing", err)
			for _, want := range []string{
				`Unable to read proxy config "billing": `, "cannot unmarshal number",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("APIErrorDetail = %q, want it to contain %q", got, want)
				}
			}
			for _, hidden := range []string{secret, "200 OK"} {
				if strings.Contains(got, hidden) {
					t.Errorf("APIErrorDetail = %q, want it without %q", got, hidden)
				}
			}
		})
	}
}

// notFoundBody is what the fakes answer every error with, so that each error's
// message says "not found" whatever its status.
const notFoundBody = `{"error_code":"NOT_FOUND","message":"object not found"}`

// sdkError returns the error the SDK makes for a response with this status, the
// way its API calls do.
func sdkError(t *testing.T, status int) error {
	t.Helper()
	return sdkErrorWithBody(t, status, notFoundBody)
}

// sdkErrorWithBody returns the error the SDK makes for a response with this status
// and body.
func sdkErrorWithBody(t *testing.T, status int, body string) error {
	t.Helper()
	err := permitErrors.HttpErrorHandle(errors.New("object not found"), &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	})
	if err == nil {
		t.Fatalf("the SDK made no error for status %d", status)
	}
	return err
}

// openAPIError returns the error the SDK's generated client returns for a response
// with this status, which the SDK passes on unwrapped from ResourceAttributes.Get.
func openAPIError(t *testing.T, status int) error {
	t.Helper()
	_, err := openAPIResponse(t, status, notFoundBody)
	return err
}

// openAPIResponse sends the generated client's ResourceAttributes.Get to a server
// that answers with this status and body, and returns the response and the error,
// which must be a *openapi.GenericOpenAPIError.
func openAPIResponse(t *testing.T, status int, body string) (*http.Response, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()
	config := openapi.NewConfiguration()
	config.Servers = openapi.ServerConfigurations{{URL: server.URL}}
	_, response, err := openapi.NewAPIClient(config).ResourceAttributesApi.
		GetResourceAttribute(context.Background(), "proj", "env", "__user", "tier").Execute()
	var openAPIErr *openapi.GenericOpenAPIError
	if !errors.As(err, &openAPIErr) {
		t.Fatalf("status %d: got error %v (%T), want a *openapi.GenericOpenAPIError",
			status, err, err)
	}
	return response, err
}
