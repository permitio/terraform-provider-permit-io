terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# A top-level role that is managed outside this configuration.
data "permitio_role" "viewer" {
  key = "viewer"
}

# A role on the document resource.
data "permitio_role" "document_owner" {
  key      = "owner"
  resource = "document"
}

resource "permitio_role_assignment" "jane_viewer" {
  user   = "jane@example.com"
  role   = data.permitio_role.viewer.key
  tenant = "default"
}

output "document_owner_permissions" {
  value = data.permitio_role.document_owner.permissions
}
