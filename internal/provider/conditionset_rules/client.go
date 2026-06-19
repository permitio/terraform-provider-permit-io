package conditionsetrules

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/permit"
)

type ConditionSetRuleModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	UserSet        types.String `tfsdk:"user_set"`
	Permission     types.String `tfsdk:"permission"`
	ResourceSet    types.String `tfsdk:"resource_set"`
}

type ConditionSetRuleClient struct {
	client *permit.Client
}

func (c *ConditionSetRuleClient) Read(ctx context.Context, data ConditionSetRuleModel) (ConditionSetRuleModel, error) {
	rules, err := c.client.Api.ConditionSets.ListSetPermissions(
		ctx,
		data.UserSet.ValueString(),
		data.Permission.ValueString(),
		data.ResourceSet.ValueString(),
	)

	if err != nil {
		return ConditionSetRuleModel{}, err
	}

	// The list is filtered server-side by user_set/permission/resource_set, so an
	// empty result means the rule was removed outside of Terraform.
	if len(rules) == 0 {
		return ConditionSetRuleModel{}, fmt.Errorf("condition set rule not found")
	}

	rule := rules[0]
	data.Id = types.StringValue(rule.Id)
	data.OrganizationId = types.StringValue(rule.OrganizationId)
	data.ProjectId = types.StringValue(rule.ProjectId)
	data.EnvironmentId = types.StringValue(rule.EnvironmentId)

	return data, nil
}

func (c *ConditionSetRuleClient) Create(ctx context.Context, rulePlan *ConditionSetRuleModel) error {
	ruleRead, err := c.client.Api.ConditionSets.AssignSetPermissions(
		ctx,
		rulePlan.UserSet.ValueString(),
		rulePlan.Permission.ValueString(),
		rulePlan.ResourceSet.ValueString())

	if err != nil {
		return err
	}

	rulePlan.Id = types.StringValue(ruleRead[0].Id)
	rulePlan.OrganizationId = types.StringValue(ruleRead[0].OrganizationId)
	rulePlan.ProjectId = types.StringValue(ruleRead[0].ProjectId)
	rulePlan.EnvironmentId = types.StringValue(ruleRead[0].EnvironmentId)

	return nil
}

func (c *ConditionSetRuleClient) Delete(ctx context.Context, rulePlan *ConditionSetRuleModel) error {
	return c.client.Api.ConditionSets.UnassignSetPermissions(
		ctx,
		rulePlan.UserSet.ValueString(),
		rulePlan.Permission.ValueString(),
		rulePlan.ResourceSet.ValueString(),
	)
}
