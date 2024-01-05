package common

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

func CreateBaseResourceSchema() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The resource ID. This is a unique identifier for the resource. ",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"key": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The key. This is a unique identifier. ",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "The name. This is a human-readable name for the object. ",
			Required:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "The description. This is a human-readable description for the object. ",
			Optional:            true,
			Computed:            true,
		},
		"organization_id": schema.StringAttribute{
			MarkdownDescription: "The organization ID. This is a unique identifier for the organization. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"project_id": schema.StringAttribute{
			MarkdownDescription: "The project ID. This is a unique identifier for the project. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"environment_id": schema.StringAttribute{
			MarkdownDescription: "The environment ID. This is a unique identifier for the environment. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"created_at": schema.StringAttribute{
			MarkdownDescription: "The creation timestamp. This is a timestamp for when the object was created. ",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"updated_at": schema.StringAttribute{
			MarkdownDescription: "The update timestamp. This is a timestamp for when the object was last updated. ",
			Optional:            true,
			Computed:            true,
		},
	}
}
