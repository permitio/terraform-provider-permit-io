package users

import (
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type userModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	Key            types.String `tfsdk:"key"`
	Email          types.String `tfsdk:"email"`
	FirstName      types.String `tfsdk:"first_name"`
	LastName       types.String `tfsdk:"last_name"`
	Attributes     types.String `tfsdk:"attributes"`
}

func tfModelFromUserRead(m models.UserRead) userModel {
	r := userModel{}
	r.Id = types.StringValue(m.Id)
	r.Key = types.StringValue(m.Key)
	r.Email = types.StringPointerValue(m.Email)
	r.FirstName = types.StringPointerValue(m.FirstName)
	r.LastName = types.StringPointerValue(m.LastName)
	r.EnvironmentId = types.StringValue(m.EnvironmentId)
	r.ProjectId = types.StringValue(m.ProjectId)
	r.OrganizationId = types.StringValue(m.OrganizationId)

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
