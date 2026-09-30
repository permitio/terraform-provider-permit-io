terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

resource "permitio_resource" "document" {
  key  = "document"
  name = "Document"
  actions = {
    read   = { name = "Read" }
    write  = { name = "Write" }
    delete = { name = "Delete" }
  }
}

# A top-level role grants permissions as resource_key:action_key. Building them
# from the resource's key makes Terraform create the resource first.
resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Reads and writes documents"
  permissions = [
    "${permitio_resource.document.key}:read",
    "${permitio_resource.document.key}:write",
  ]
}

# An admin has the editor's permissions and can also delete documents.
resource "permitio_role" "admin" {
  key         = "admin"
  name        = "Admin"
  permissions = ["${permitio_resource.document.key}:delete"]
  extends     = [permitio_role.editor.key]
}

# A resource role belongs to one resource and grants its action keys.
resource "permitio_role" "document_owner" {
  key         = "owner"
  name        = "Owner"
  resource    = permitio_resource.document.key
  permissions = ["read", "write", "delete"]
}
