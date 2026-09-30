package proxy_configs

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

// uniqueMappingRulesValidator rejects two mapping rules with the same url and
// http_method, which the API takes as one rule: a create stores both, but an
// update keeps one, so the state could never match the API. It skips a rule whose
// url or http_method is not known yet.
type uniqueMappingRulesValidator struct{}

func (v uniqueMappingRulesValidator) Description(_ context.Context) string {
	return "no two mapping rules may have the same url and http_method"
}

func (v uniqueMappingRulesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v uniqueMappingRulesValidator) ValidateList(
	_ context.Context, request validator.ListRequest, response *validator.ListResponse,
) {
	if request.ConfigValue.IsUnknown() || request.ConfigValue.IsNull() {
		return
	}

	first := map[mappingRuleKey]int{}
	for i, element := range request.ConfigValue.Elements() {
		rule, ok := element.(types.Object)
		if !ok || rule.IsUnknown() || rule.IsNull() {
			continue
		}
		url, urlOK := rule.Attributes()["url"].(types.String)
		httpMethod, methodOK := rule.Attributes()["http_method"].(types.String)
		if !urlOK || !methodOK || url.IsUnknown() || httpMethod.IsUnknown() {
			continue
		}
		key := mappingRuleKey{url: url.ValueString(), httpMethod: httpMethod.ValueString()}
		if j, seen := first[key]; seen {
			response.Diagnostics.AddAttributeError(
				request.Path.AtListIndex(i),
				"Duplicate mapping rule",
				fmt.Sprintf("mapping_rules[%d] has the same url %q and http_method %q as "+
					"mapping_rules[%d]. The Permit API identifies a mapping rule by its url and "+
					"http_method, so each pair must be unique.", i, key.url, key.httpMethod, j),
			)
			continue
		}
		first[key] = i
	}
}
