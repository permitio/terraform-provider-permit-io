---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "permitio_relation Resource - terraform-provider-permit-io"
subcategory: ""
description: |-
  See the documentation https://api.permit.io/v2/redoc#tag/Resource-Relations/operation/create_resource_relation for more information about Relations
---

# permitio_relation (Resource)

See [the documentation](https://api.permit.io/v2/redoc#tag/Resource-Relations/operation/create_resource_relation) for more information about Relations



<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `key` (String) The key. This is a unique identifier.
- `name` (String) The name. This is a human-readable name for the object.
- `object_resource` (String) The object resource ID or key
- `subject_resource` (String) The subject resource ID or key

### Optional

- `description` (String) The description. This is a human-readable description for the object.
- `updated_at` (String) The update timestamp. This is a timestamp for when the object was last updated.

### Read-Only

- `created_at` (String) The creation timestamp. This is a timestamp for when the object was created.
- `environment_id` (String) The environment ID. This is a unique identifier for the environment.
- `id` (String) The resource ID. This is a unique identifier for the resource.
- `object_resource_id` (String) The object resource ID
- `organization_id` (String) The organization ID. This is a unique identifier for the organization.
- `project_id` (String) The project ID. This is a unique identifier for the project.
- `subject_resource_id` (String) The subject resource ID
