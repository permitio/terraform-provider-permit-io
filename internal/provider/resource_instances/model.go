package resource_instances

import (
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type resourceInstanceModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
	Key            types.String `tfsdk:"key"`
	Resource       types.String `tfsdk:"resource"`
	ResourceId     types.String `tfsdk:"resource_id"`
	Tenant         types.String `tfsdk:"tenant"`
	Attributes     types.String `tfsdk:"attributes"`
}

func tfModelFromResourceInstanceRead(m models.ResourceInstanceRead) resourceInstanceModel {
	r := resourceInstanceModel{}
	r.Id = types.StringValue(m.Id)
	r.Key = types.StringValue(m.Key)
	r.Resource = types.StringValue(m.Resource)
	r.ResourceId = types.StringValue(m.ResourceId)
	r.OrganizationId = types.StringValue(m.OrganizationId)
	r.ProjectId = types.StringValue(m.ProjectId)
	r.EnvironmentId = types.StringValue(m.EnvironmentId)
	r.CreatedAt = types.StringValue(m.CreatedAt.String())
	r.UpdatedAt = types.StringValue(m.UpdatedAt.String())
	r.Tenant = types.StringPointerValue(m.Tenant)

	// Convert attributes map to JSON string
	if len(m.Attributes) > 0 {
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
