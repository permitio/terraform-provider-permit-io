package tenants

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

type tenantModel struct {
	Id             types.String         `tfsdk:"id"`
	OrganizationId types.String         `tfsdk:"organization_id"`
	ProjectId      types.String         `tfsdk:"project_id"`
	EnvironmentId  types.String         `tfsdk:"environment_id"`
	CreatedAt      types.String         `tfsdk:"created_at"`
	UpdatedAt      types.String         `tfsdk:"updated_at"`
	LastActionAt   types.String         `tfsdk:"last_action_at"`
	Key            types.String         `tfsdk:"key"`
	Name           types.String         `tfsdk:"name"`
	Description    types.String         `tfsdk:"description"`
	Attributes     jsontypes.Normalized `tfsdk:"attributes"`
}

// tfModelFromTenantRead returns the model of the tenant the API answered with.
// priorAttributes is the attributes of the plan or the prior state, which the
// model keeps when the API holds the same attributes; see
// common.OptionalJSONObjectValue.
func tfModelFromTenantRead(m models.TenantRead, priorAttributes jsontypes.Normalized,
) (tenantModel, error) {
	r := tenantModel{}
	r.Id = types.StringValue(m.Id)
	r.Key = types.StringValue(m.Key)
	r.Name = types.StringValue(m.Name)
	r.Description = types.StringPointerValue(m.Description)
	r.EnvironmentId = types.StringValue(m.EnvironmentId)
	r.ProjectId = types.StringValue(m.ProjectId)
	r.OrganizationId = types.StringValue(m.OrganizationId)
	r.CreatedAt = types.StringValue(m.CreatedAt.String())
	r.UpdatedAt = types.StringValue(m.UpdatedAt.String())
	r.LastActionAt = types.StringValue(m.LastActionAt.String())

	attributes, err := common.OptionalJSONObjectValue(m.Attributes, priorAttributes)
	if err != nil {
		return tenantModel{}, fmt.Errorf("encoding the attributes: %w", err)
	}
	r.Attributes = attributes

	return r, nil
}
