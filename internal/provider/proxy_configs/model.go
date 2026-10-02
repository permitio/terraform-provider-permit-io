package proxy_configs

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type mappingRuleModel struct {
	Url        types.String `tfsdk:"url"`
	UrlType    types.String `tfsdk:"url_type"`
	HttpMethod types.String `tfsdk:"http_method"`
	Resource   types.String `tfsdk:"resource"`
	Action     types.String `tfsdk:"action"`
	Priority   types.Int64  `tfsdk:"priority"`
	Headers    types.Map    `tfsdk:"headers"`
}

type authSecretModel struct {
	Basic   types.String            `tfsdk:"basic"`
	Bearer  types.String            `tfsdk:"bearer"`
	Headers map[string]types.String `tfsdk:"headers"`
}

type proxyConfigModel struct {
	Id             types.String       `tfsdk:"id"`
	OrganizationId types.String       `tfsdk:"organization_id"`
	ProjectId      types.String       `tfsdk:"project_id"`
	EnvironmentId  types.String       `tfsdk:"environment_id"`
	Key            types.String       `tfsdk:"key"`
	Name           types.String       `tfsdk:"name"`
	AuthMechanism  types.String       `tfsdk:"auth_mechanism"`
	AuthSecret     authSecretModel    `tfsdk:"auth_secret"`
	MappingRules   []mappingRuleModel `tfsdk:"mapping_rules"`
}

func (model *proxyConfigModel) toProxyConfigCreate(ctx context.Context) (models.ProxyConfigCreate, error) {
	authMech := models.AuthMechanism(model.AuthMechanism.ValueString())
	mappingRules := make([]models.MappingRule, len(model.MappingRules))

	for i, rule := range model.MappingRules {
		mappingRules[i] = models.MappingRule{
			Url:        rule.Url.ValueString(),
			HttpMethod: models.Methods(rule.HttpMethod.ValueString()),
			Resource:   rule.Resource.ValueString(),
			Action:     rule.Action.ValueStringPointer(),
		}

		if !rule.UrlType.IsNull() {
			urlType := models.UrlMatchType(rule.UrlType.ValueString())
			mappingRules[i].UrlType = &urlType
		}

		if !rule.Priority.IsNull() {
			priority := int32(rule.Priority.ValueInt64())
			mappingRules[i].Priority = &priority
		}

		if !rule.Headers.IsNull() {
			headers := make(map[string]string)

			for headerKey, headerValue := range rule.Headers.Elements() {
				tfValue, err := headerValue.ToTerraformValue(ctx)

				if err != nil {
					return models.ProxyConfigCreate{}, err
				}

				var strValue string
				err = tfValue.As(&strValue)

				if err != nil {
					return models.ProxyConfigCreate{}, err
				}

				headers[headerKey] = strValue
			}

			mappingRules[i].Headers = &headers
		}
	}

	proxyConfigCreate := models.ProxyConfigCreate{
		Key:           model.Key.ValueString(),
		Name:          model.Name.ValueString(),
		AuthMechanism: &authMech,
		MappingRules:  mappingRules,
	}

	switch models.AuthMechanism(model.AuthMechanism.ValueString()) {
	case models.BASIC:
		proxyConfigCreate.Secret = model.AuthSecret.Basic.ValueString()
	case models.BEARER:
		proxyConfigCreate.Secret = model.AuthSecret.Bearer.ValueString()
	}

	return proxyConfigCreate, nil
}

// proxyConfigPatch is the body of a proxy config update. The API rejects an update
// without the secret, and takes a missing auth_mechanism as Bearer, so the body
// always has both. The API merges the mapping rules of an update into the stored
// ones by url and http_method, in the order sent: it removes the stored rule with
// the pair of a removedMappingRule, replaces the stored rule with the pair of any
// other rule in its place or adds that rule at the end, and keeps every stored
// rule the update does not name.
type proxyConfigPatch struct {
	Name          string `json:"name"`
	AuthMechanism string `json:"auth_mechanism"`
	Secret        string `json:"secret"`
	// MappingRules holds a removedMappingRule for each rule of the prior state and
	// then a models.MappingRule for each planned rule, so the API ends up with the
	// planned rules in the planned order. It is never nil: the API rejects null.
	MappingRules []any `json:"mapping_rules"`
}

// removedMappingRule asks the API to remove the stored mapping rule with this url
// and http_method. The API requires a resource on every rule, even one it removes.
type removedMappingRule struct {
	Url          string `json:"url"`
	HttpMethod   string `json:"http_method"`
	Resource     string `json:"resource"`
	ShouldDelete bool   `json:"should_delete"`
}

// toProxyConfigPatch returns the update that makes the proxy config in prior
// state the planned one: the planned name, auth mechanism and secret, a
// removedMappingRule for each url and http_method in prior, and then the planned
// mapping rules. The API removes every prior rule and then adds each planned one
// at the end, so one request leaves it with exactly the planned rules in the
// planned order.
func (model *proxyConfigModel) toProxyConfigPatch(
	ctx context.Context, prior proxyConfigModel,
) (proxyConfigPatch, error) {
	created, err := model.toProxyConfigCreate(ctx)

	if err != nil {
		return proxyConfigPatch{}, err
	}

	rules := make([]any, 0, len(prior.MappingRules)+len(created.MappingRules))
	removed := map[mappingRuleKey]bool{}
	for _, rule := range prior.MappingRules {
		key := rule.key()
		if removed[key] {
			continue
		}
		removed[key] = true
		rules = append(rules, removedMappingRule{
			Url:          key.url,
			HttpMethod:   key.httpMethod,
			Resource:     rule.Resource.ValueString(),
			ShouldDelete: true,
		})
	}
	for _, rule := range created.MappingRules {
		rules = append(rules, rule)
	}

	return proxyConfigPatch{
		Name:          created.Name,
		AuthMechanism: model.AuthMechanism.ValueString(),
		Secret:        created.Secret,
		MappingRules:  rules,
	}, nil
}

// mappingRuleKey is what the API identifies a mapping rule by: its url and
// http_method.
type mappingRuleKey struct {
	url, httpMethod string
}

func (rule mappingRuleModel) key() mappingRuleKey {
	return mappingRuleKey{url: rule.Url.ValueString(), httpMethod: rule.HttpMethod.ValueString()}
}

// orderMappingRules returns rules, as the API returned them, in the order of the
// same rules in want, matched by url and http_method, followed by the rules want
// does not have, in the API's order. An update leaves the API with the planned
// order, but a change made outside Terraform can leave it with another. The state
// keeps the configuration's order, and a rule only the API has goes at the end,
// where the next plan removes it.
func orderMappingRules(rules, want []mappingRuleModel) []mappingRuleModel {
	used := make([]bool, len(rules))
	ordered := make([]mappingRuleModel, 0, len(rules))
	for _, wanted := range want {
		for i, rule := range rules {
			if !used[i] && rule.key() == wanted.key() {
				used[i] = true
				ordered = append(ordered, rule)
				break
			}
		}
	}
	for i, rule := range rules {
		if !used[i] {
			ordered = append(ordered, rule)
		}
	}
	return ordered
}

func (model *proxyConfigModel) fromProxyConfigRead(sdkModel *models.ProxyConfigRead) {
	model.Id = types.StringValue(sdkModel.Id)
	model.OrganizationId = types.StringValue(sdkModel.OrganizationId)
	model.ProjectId = types.StringValue(sdkModel.ProjectId)
	model.EnvironmentId = types.StringValue(sdkModel.EnvironmentId)
	model.Key = types.StringValue(sdkModel.Key)
	model.Name = types.StringValue(sdkModel.Name)
	// A response without auth_mechanism has the API's default, Bearer.
	authMechanism := models.BEARER
	if sdkModel.AuthMechanism != nil {
		authMechanism = *sdkModel.AuthMechanism
	}
	model.AuthMechanism = types.StringValue(string(authMechanism))

	switch authMechanism {
	case models.BASIC:
		model.AuthSecret.Basic = types.StringValue(sdkModel.Secret)
	case models.BEARER:
		model.AuthSecret.Bearer = types.StringValue(sdkModel.Secret)
	}

	resultRules := make([]mappingRuleModel, len(sdkModel.MappingRules))

	for i, rule := range sdkModel.MappingRules {
		resultRules[i] = mappingRuleModel{
			Url:        types.StringValue(rule.Url),
			HttpMethod: types.StringValue(string(rule.HttpMethod)),
			Resource:   types.StringValue(rule.Resource),
		}

		if rule.IsRegexUrl() {
			resultRules[i].UrlType = types.StringValue(string(models.URLMatchTypeRegex))
		} else {
			resultRules[i].UrlType = types.StringNull()
		}

		if rule.Action != nil {
			resultRules[i].Action = types.StringPointerValue(rule.Action)
		} else {
			resultRules[i].Action = types.StringNull()
		}

		if rule.Priority != nil {
			priority := int64(*rule.Priority)
			resultRules[i].Priority = types.Int64Value(priority)
		} else {
			resultRules[i].Priority = types.Int64Null()
		}

		if rule.Headers != nil && len(*rule.Headers) > 0 {
			headers := make(map[string]attr.Value)

			for headerKey, headerValue := range *rule.Headers {
				headers[headerKey] = types.StringValue(headerValue)
			}

			resultRules[i].Headers = types.MapValueMust(types.StringType, headers)
		} else {
			resultRules[i].Headers = types.MapNull(types.StringType)
		}
	}

	model.MappingRules = resultRules
}
