package common

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// LookupOnlyInputNote ends the description of each data source attribute that a
// configuration may set but that does not select what the data source reads. Such
// attributes are accepted so that older configurations keep working.
const LookupOnlyInputNote = "Setting it does not filter the lookup: Permit's value replaces it."

func CreateBaseResourceSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The ID Permit assigns to the object.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"key": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The key that identifies the object. Changing it replaces the object.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "The name. This is a human-readable name for the object. ",
			Required:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "A description of the object; leaving it out keeps the one in Permit.",
			Optional:            true,
			Computed:            true,
		},
		"organization_id": schema.StringAttribute{
			MarkdownDescription: "The organization ID. This is a unique identifier for the organization. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"project_id": schema.StringAttribute{
			MarkdownDescription: "The project ID. This is a unique identifier for the project. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"environment_id": schema.StringAttribute{
			MarkdownDescription: "The environment ID. This is a unique identifier for the environment. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"created_at": schema.StringAttribute{
			MarkdownDescription: "The creation timestamp. This is a timestamp for when the object was created. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseNonNullStateForUnknown(),
			},
		},
		"updated_at": schema.StringAttribute{
			MarkdownDescription: "The update timestamp. This is a timestamp for when the object was last updated. ",
			Computed:            true,
		},
	}
}
