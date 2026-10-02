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
    read = { name = "Read" }
  }
}

resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  permissions = ["${permitio_resource.document.key}:read"]
}

resource "permitio_tenant" "acme" {
  key  = "acme"
  name = "Acme"
}

# The provider does not manage users: user is the key of a user in Permit.
resource "permitio_role_assignment" "jane_viewer" {
  user   = "jane@example.com"
  role   = permitio_role.viewer.key
  tenant = permitio_tenant.acme.key
}
