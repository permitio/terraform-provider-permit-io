package role_derivations

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/samber/lo"
)

type apiClient struct {
	client *permit.Client
}

func (c *apiClient) Create(ctx context.Context, plan roleDerivationModel) (roleDerivationModel, error) {
	derivedRuleCreate := models.DerivedRoleRuleCreate{
		Role:             plan.Role.ValueString(),
		OnResource:       plan.OnResource.ValueString(),
		LinkedByRelation: plan.LinkedByRelation.ValueString(),
	}

	createdGrant, err := c.client.Api.ImplicitGrants.Create(
		ctx,
		plan.Resource.ValueString(),
		plan.ToRole.ValueString(),
		derivedRuleCreate,
	)

	if err != nil {
		return roleDerivationModel{}, err
	}

	createdModel := tfModelFromDerivedRoleRuleRead(plan, *createdGrant)
	return createdModel, nil
}

func (c *apiClient) Read(ctx context.Context, plan roleDerivationModel) (roleDerivationModel, error) {
	targetRoleRead, err := c.client.Api.ResourceRoles.Get(
		ctx, plan.Resource.ValueString(), plan.ToRole.ValueString())

	if err != nil {
		return roleDerivationModel{},
			fmt.Errorf("failed getting target role %s/%s: %w", plan.Resource.ValueString(), plan.ToRole.ValueString(), err)
	}

	if targetRoleRead.GrantedTo == nil {
		return roleDerivationModel{}, fmt.Errorf("target role has no role grants")
	}

	derivation, found := lo.Find(targetRoleRead.GrantedTo.UsersWithRole, func(item models.DerivedRoleRuleRead) bool {
		return item.OnResource == plan.OnResource.ValueString() &&
			item.Role == plan.Role.ValueString() &&
			item.LinkedByRelation == plan.LinkedByRelation.ValueString()
	})

	if !found {
		return roleDerivationModel{},
			fmt.Errorf("derivation not found")
	}

	return tfModelFromDerivedRoleRuleRead(plan, derivation), nil
}

func (c *apiClient) Delete(ctx context.Context, plan roleDerivationModel) error {
	derivedRuleDelete := models.DerivedRoleRuleDelete{
		Role:             plan.ToRole.ValueString(),
		OnResource:       plan.OnResource.ValueString(),
		LinkedByRelation: plan.LinkedByRelation.ValueString(),
	}

	return c.client.Api.ImplicitGrants.Delete(
		ctx,
		plan.Resource.ValueString(),
		plan.Role.ValueString(),
		derivedRuleDelete)
}
