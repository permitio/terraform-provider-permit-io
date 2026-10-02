package resource_instances

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

type resourceInstanceModel struct {
	Id             types.String         `tfsdk:"id"`
	OrganizationId types.String         `tfsdk:"organization_id"`
	ProjectId      types.String         `tfsdk:"project_id"`
	EnvironmentId  types.String         `tfsdk:"environment_id"`
	CreatedAt      types.String         `tfsdk:"created_at"`
	UpdatedAt      types.String         `tfsdk:"updated_at"`
	Key            types.String         `tfsdk:"key"`
	Resource       types.String         `tfsdk:"resource"`
	ResourceId     types.String         `tfsdk:"resource_id"`
	Tenant         types.String         `tfsdk:"tenant"`
	Attributes     jsontypes.Normalized `tfsdk:"attributes"`
}

// tfModelFromResourceInstanceRead returns the model of the resource instance the
// API answered with. priorAttributes is the attributes of the plan or the prior
// state, which the model keeps when the API holds the same attributes; see
// common.OptionalJSONObjectValue.
func tfModelFromResourceInstanceRead(m models.ResourceInstanceRead,
	priorAttributes jsontypes.Normalized,
) (resourceInstanceModel, error) {
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

	attributes, err := common.OptionalJSONObjectValue(m.Attributes, priorAttributes)
	if err != nil {
		return resourceInstanceModel{}, fmt.Errorf("encoding the attributes: %w", err)
	}
	r.Attributes = attributes

	return r, nil
}
