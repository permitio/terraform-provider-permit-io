package proxy_configs

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/permitio/permit-golang/pkg/models"
)

type authMechanismValidator struct{}

func (v authMechanismValidator) Description(_ context.Context) string {
	return "auth_mechanism must be Bearer or Basic; Headers is not supported yet"
}

func (v authMechanismValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v authMechanismValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsUnknown() || request.ConfigValue.IsNull() {
		return
	}

	value := request.ConfigValue.ValueString()

	// The API takes a Headers secret as an object of header values, but the SDK
	// sends the secret as a string, so a Headers proxy config could never be
	// created or updated (PER-16605).
	if models.AuthMechanism(value) == models.HEADERS {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Unsupported auth_mechanism",
			"Headers authentication is not supported yet; use Bearer or Basic.",
		)
		return
	}

	if !models.AuthMechanism(value).IsValid() {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid auth_mechanism",
			fmt.Sprintf("%s, got %s", v.Description(ctx), value),
		)
		return
	}
}
