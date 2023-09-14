package conditionsetrules

import (
	"context"
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
	_, err := c.client.Api.ConditionSets.ListSetPermissions(
		ctx,
		data.UserSet.ValueString(),
		data.Permission.ValueString(),
		data.ResourceSet.ValueString(),
	)

	if err != nil {
		return ConditionSetRuleModel{}, err
	}

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
