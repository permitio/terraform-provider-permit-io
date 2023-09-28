package proxy_configs

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/permitio/permit-golang/pkg/models"
)

type authMechanismValidator struct{}

func (v authMechanismValidator) Description(_ context.Context) string {
	return fmt.Sprintf("auth_mechanism must be in %s", models.AllowedAuthMechanismEnumValues)
}

func (v authMechanismValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v authMechanismValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsUnknown() || request.ConfigValue.IsNull() {
		return
	}

	value := request.ConfigValue.ValueString()

	if !models.AuthMechanism(value).IsValid() {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid auth_mechanism",
			fmt.Sprintf("%s, got %s", v.Description(ctx), value),
		)
		return
	}
}
