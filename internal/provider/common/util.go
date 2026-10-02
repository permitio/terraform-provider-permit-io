package common

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func ConvertElementsToSlice[T any](ctx context.Context, elements []attr.Value) ([]T, error) {
	slice := make([]T, len(elements))

	for i, extend := range elements {
		tfValue, err := extend.ToTerraformValue(ctx)

		if err != nil {
			return nil, err
		}

		var value T
		err = tfValue.As(&value)

		if err != nil {
			return nil, err
		}

		slice[i] = value
	}

	return slice, nil
}

// KnownStringPointer returns a pointer to the value of s, or nil when s is null or
// unknown, so that a request leaves the field out. ValueStringPointer returns a
// pointer to "" for an unknown value, such as an optional and computed attribute
// the configuration leaves out, and a request would send that "".
func KnownStringPointer(s types.String) *string {
	if s.IsUnknown() {
		return nil
	}
	return s.ValueStringPointer()
}
