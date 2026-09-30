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
    read  = { name = "Read" }
    write = { name = "Write" }
  }
}

resource "permitio_role" "document_editor" {
  key         = "editor"
  name        = "Editor"
  resource    = permitio_resource.document.key
  permissions = ["read", "write"]
}

resource "permitio_tenant" "acme" {
  key  = "acme"
  name = "Acme"
}

resource "permitio_resource_instance" "quarterly_report" {
  key      = "quarterly-report"
  resource = permitio_resource.document.key
  tenant   = permitio_tenant.acme.key
}

# Jane can edit this one document. The provider does not manage users: user is
# the key of a user in Permit.
resource "permitio_resource_instance_role_assignment" "jane_edits_quarterly_report" {
  user              = "jane@example.com"
  role              = permitio_role.document_editor.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.quarterly_report.key
  tenant            = permitio_tenant.acme.key
}
