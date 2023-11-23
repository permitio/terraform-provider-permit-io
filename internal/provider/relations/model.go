package relations

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
)

type relationModel struct {
	Id             types.String `tfsdk:"id"`
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	OrganizationId types.String `tfsdk:"organization_id"`

	SubjectResource   types.String `tfsdk:"subject_resource"`
	ObjectResource    types.String `tfsdk:"object_resource"`
	SubjectResourceId types.String `tfsdk:"subject_resource_id"`
	ObjectResourceId  types.String `tfsdk:"object_resource_id"`
}

var invalidModel = relationModel{}

func tfModelFromSDK(m models.RelationRead) relationModel {
	r := relationModel{}
	r.Id = types.StringValue(m.Id)
	r.Key = types.StringValue(m.Key)
	r.Name = types.StringValue(m.Name)
	r.Description = types.StringPointerValue(m.Description)
	r.SubjectResourceId = types.StringValue(m.SubjectResourceId)
	r.SubjectResource = types.StringValue(m.SubjectResource)
	r.ObjectResourceId = types.StringValue(m.ObjectResourceId)
	r.ObjectResource = types.StringValue(m.ObjectResource)
	r.EnvironmentId = types.StringValue(m.EnvironmentId)
	r.ProjectId = types.StringValue(m.ProjectId)
	r.OrganizationId = types.StringValue(m.OrganizationId)

	return r
}
