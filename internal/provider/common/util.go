package common

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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