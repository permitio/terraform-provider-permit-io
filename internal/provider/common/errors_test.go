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

// notFoundBody is what the fakes answer every error with, so that each error's
// message says "not found" whatever its status.
const notFoundBody = `{"error_code":"NOT_FOUND","message":"object not found"}`

// sdkError returns the error the SDK makes for a response with this status, the
// way its API calls do.
func sdkError(t *testing.T, status int) error {
	t.Helper()
	err := permitErrors.HttpErrorHandle(errors.New("object not found"), &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(notFoundBody)),
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, notFoundBody)
	}))
	defer server.Close()
	config := openapi.NewConfiguration()
	config.Servers = openapi.ServerConfigurations{{URL: server.URL}}
	_, _, err := openapi.NewAPIClient(config).ResourceAttributesApi.
		GetResourceAttribute(context.Background(), "proj", "env", "__user", "tier").Execute()
	var openAPIErr *openapi.GenericOpenAPIError
	if !errors.As(err, &openAPIErr) {
		t.Fatalf("status %d: got error %v (%T), want a *openapi.GenericOpenAPIError",
			status, err, err)
	}
	return err
}
