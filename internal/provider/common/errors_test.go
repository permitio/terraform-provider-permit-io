package common

import (
	"errors"
	"testing"
)

func TestIsNotFoundErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		// The shape the SDK returns on a 404 - note the mixed casing.
		{"api 404", errors.New("ErrorCode: NotFound, ErrorType: API_ERROR, Message: 404 Not Found"), true},
		{"synthesized", errors.New("role assignment not found"), true},
		{"unrelated", errors.New("ErrorCode: Forbidden, Message: 403 Forbidden"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFoundErr(tt.err); got != tt.want {
				t.Errorf("IsNotFoundErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
