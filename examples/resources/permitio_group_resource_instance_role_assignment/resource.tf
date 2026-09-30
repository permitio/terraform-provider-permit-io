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

resource "permitio_role" "document_viewer" {
  key         = "viewer"
  name        = "Viewer"
  resource    = permitio_resource.document.key
  permissions = ["read"]
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

# Every member of the finance group can view this one document. The provider does
# not manage groups: group is the key of a group in Permit.
resource "permitio_group_resource_instance_role_assignment" "finance_views_quarterly_report" {
  group             = "finance"
  role              = permitio_role.document_viewer.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.quarterly_report.key
  tenant            = permitio_tenant.acme.key
}
