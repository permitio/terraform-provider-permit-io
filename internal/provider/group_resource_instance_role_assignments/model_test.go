package group_resource_instance_role_assignments

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGroupAddRoleJSON(t *testing.T) {
	addRole := GroupAddRole{
		Role:             "admin",
		Resource:         "workspace",
		ResourceInstance: "ws-123",
		Tenant:           "default",
	}

	if addRole.Role != "admin" {
		t.Errorf("Role = %v, want admin", addRole.Role)
	}
	if addRole.Resource != "workspace" {
		t.Errorf("Resource = %v, want workspace", addRole.Resource)
	}
	if addRole.ResourceInstance != "ws-123" {
		t.Errorf("ResourceInstance = %v, want ws-123", addRole.ResourceInstance)
	}
	if addRole.Tenant != "default" {
		t.Errorf("Tenant = %v, want default", addRole.Tenant)
	}
}

func TestGroupResourceInstanceRoleAssignmentModel(t *testing.T) {
	model := GroupResourceInstanceRoleAssignmentModel{
		Id:               types.StringValue("test-id"),
		Group:            types.StringValue("developers"),
		Role:             types.StringValue("admin"),
		Resource:         types.StringValue("workspace"),
		ResourceInstance: types.StringValue("ws-123"),
		Tenant:           types.StringValue("default"),
	}

	if model.Id.ValueString() != "test-id" {
		t.Errorf("Id = %v, want test-id", model.Id.ValueString())
	}
	if model.Group.ValueString() != "developers" {
		t.Errorf("Group = %v, want developers", model.Group.ValueString())
	}
	if model.Role.ValueString() != "admin" {
		t.Errorf("Role = %v, want admin", model.Role.ValueString())
	}
	if model.Resource.ValueString() != "workspace" {
		t.Errorf("Resource = %v, want workspace", model.Resource.ValueString())
	}
	if model.ResourceInstance.ValueString() != "ws-123" {
		t.Errorf("ResourceInstance = %v, want ws-123", model.ResourceInstance.ValueString())
	}
	if model.Tenant.ValueString() != "default" {
		t.Errorf("Tenant = %v, want default", model.Tenant.ValueString())
	}
}
