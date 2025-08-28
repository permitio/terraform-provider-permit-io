package proxy_configs

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type mappingRuleModel struct {
	Url        types.String `tfsdk:"url"`
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
	case models.HEADERS:
	}

	return proxyConfigCreate, nil
}

func (model *proxyConfigModel) toProxyConfigUpdate(ctx context.Context) (models.ProxyConfigUpdate, error) {
	created, err := model.toProxyConfigCreate(ctx)

	if err != nil {
		return models.ProxyConfigUpdate{}, err
	}

	return models.ProxyConfigUpdate{
		Name:          &created.Name,
		Secret:        &created.Secret,
		AuthMechanism: created.AuthMechanism,
		MappingRules:  created.MappingRules,
	}, nil
}

func (model *proxyConfigModel) fromProxyConfigRead(sdkModel *models.ProxyConfigRead) {
	model.Id = types.StringValue(sdkModel.Id)
	model.OrganizationId = types.StringValue(sdkModel.OrganizationId)
	model.ProjectId = types.StringValue(sdkModel.ProjectId)
	model.EnvironmentId = types.StringValue(sdkModel.EnvironmentId)
	model.Key = types.StringValue(sdkModel.Key)
	model.Name = types.StringValue(sdkModel.Name)
	model.AuthMechanism = types.StringValue(string(*sdkModel.AuthMechanism))

	switch *sdkModel.AuthMechanism {
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
