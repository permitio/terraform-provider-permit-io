package tenants

import (
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type tenantModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
	LastActionAt   types.String `tfsdk:"last_action_at"`
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Attributes     types.String `tfsdk:"attributes"`
}

func tfModelFromTenantRead(m models.TenantRead) tenantModel {
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

	// Convert attributes map to JSON string
	if m.Attributes != nil && len(m.Attributes) > 0 {
		attributesJSON, err := json.Marshal(m.Attributes)
		if err == nil {
			r.Attributes = types.StringValue(string(attributesJSON))
		} else {
			r.Attributes = types.StringValue("{}")
		}
	} else {
		r.Attributes = types.StringNull()
	}

	return r
}
