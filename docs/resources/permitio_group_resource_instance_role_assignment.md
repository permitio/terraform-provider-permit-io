---
page_title: "permitio_group_resource_instance_role_assignment Resource - terraform-provider-permit-io"
subcategory: ""
description: |-
  Manages group-level role assignments on specific resource instances in Permit.io
---

# permitio_group_resource_instance_role_assignment (Resource)

Assigns a role to a group on a specific resource instance within a tenant. This allows all members of the group to inherit the specified role on that resource instance.

This resource uses the Permit.io Groups API to create both the relationship tuple and role derivation automatically, enabling group-based permissions on resource instances.

## Example Usage

```terraform
# Assign read-only role to developers group on a specific workspace
resource "permitio_group_resource_instance_role_assignment" "dev_workspace" {
  group             = "developers"
  role              = "read-only"
  resource          = "workspace"
  resource_instance = "engineering-ws"
  tenant            = "default"
}

# Multiple group assignments on the same resource instance
resource "permitio_group_resource_instance_role_assignment" "admin_workspace" {
  group             = "admins"
  role              = "admin"
  resource          = "workspace"
  resource_instance = "engineering-ws"
  tenant            = "default"
}

# Group assignment on a document resource
resource "permitio_group_resource_instance_role_assignment" "editors_doc" {
  group             = "editors"
  role              = "editor"
  resource          = "document"
  resource_instance = "project-plan-doc"
  tenant            = "default"
}
```

## Argument Reference

The following arguments are supported:

* `group` - (Required, Forces new resource) The key of the group to assign the role to. This should be an existing group in your Permit.io environment.
* `role` - (Required, Forces new resource) The key of the role to assign. This should be an existing role defined for the resource type.
* `resource` - (Required, Forces new resource) The resource type (e.g., "workspace", "document", "project"). This should match the resource type where the role is defined.
* `resource_instance` - (Required, Forces new resource) The key of the specific resource instance (e.g., "engineering-ws", "doc-123").
* `tenant` - (Required, Forces new resource) The tenant key where the assignment applies.

**Note:** All fields require replacement on change, as updating these values would effectively create a different assignment.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The identifier of the role assignment.

## Import

Group resource instance role assignments can be imported using the format `group:role:resource:resource_instance:tenant`:

```shell
terraform import permitio_group_resource_instance_role_assignment.example "developers:read-only:workspace:engineering-ws:default"
```

## How It Works

When you create this resource, Permit.io automatically:

1. **Creates a relationship tuple** - Links the group to the resource instance with the specified role
2. **Creates a role derivation** - Configures the system so that group members automatically inherit the role on the resource instance

This means all users who are members of the group will automatically receive the specified permissions on the resource instance.

## Use Cases

### Multi-tenant Applications

Grant department groups access to their specific workspace:

```terraform
resource "permitio_group_resource_instance_role_assignment" "sales_workspace" {
  group             = "sales-team"
  role              = "member"
  resource          = "workspace"
  resource_instance = "sales-workspace"
  tenant            = "acme-corp"
}
```

### Document Permissions

Give editor groups access to specific documents:

```terraform
resource "permitio_group_resource_instance_role_assignment" "legal_docs" {
  group             = "legal-team"
  role              = "editor"
  resource          = "document"
  resource_instance = "compliance-policy"
  tenant            = "default"
}
```

### Project Access Control

Assign project groups to specific project instances:

```terraform
resource "permitio_group_resource_instance_role_assignment" "project_team" {
  group             = "project-alpha-team"
  role              = "contributor"
  resource          = "project"
  resource_instance = "project-alpha"
  tenant            = "default"
}
```

## Comparison with Other Resources

* **`permitio_resource_instance_role_assignment`** - Use this for assigning roles to individual **users** on resource instances
* **`permitio_role_assignment`** - Use this for assigning tenant-level roles to users or groups
* **`permitio_group_resource_instance_role_assignment`** - Use this for assigning roles to **groups** on specific resource instances (this resource)

## Requirements

Before using this resource, ensure:

1. The group exists in your Permit.io environment
2. The resource type is defined in your schema
3. The role exists and is defined for the resource type
4. The resource instance exists
5. The tenant exists
6. At least one tenant must exist in your environment for the provider to determine the project and environment context

## Notes

* Changes to any field will destroy and recreate the resource
* The role derivation created by this resource may be shared across multiple group assignments
* Deleting this resource removes the relationship tuple but leaves the role derivation intact (as it may be used by other assignments)
