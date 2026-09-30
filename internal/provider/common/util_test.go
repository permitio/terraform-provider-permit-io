package common

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestKnownStringPointer checks that a null or unknown value gives nil, which a
// request leaves out, and that a known value, "" included, gives a pointer to it.
func TestKnownStringPointer(t *testing.T) {
	for _, tt := range []struct {
		value types.String
		want  *string
	}{
		{value: types.StringNull(), want: nil},
		{value: types.StringUnknown(), want: nil},
		{value: types.StringValue(""), want: new("")},
		{value: types.StringValue("A text document"), want: new("A text document")},
	} {
		got := KnownStringPointer(tt.value)
		switch {
		case tt.want == nil && got != nil:
			t.Errorf("KnownStringPointer(%s) = %q, want nil", tt.value, *got)
		case tt.want != nil && (got == nil || *got != *tt.want):
			t.Errorf("KnownStringPointer(%s) = %v, want a pointer to %q", tt.value, got,
				*tt.want)
		}
	}
}
