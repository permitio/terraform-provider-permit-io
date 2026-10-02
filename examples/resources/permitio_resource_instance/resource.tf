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
  attributes = {
    classified = { type = "bool" }
  }
}

resource "permitio_tenant" "acme" {
  key  = "acme"
  name = "Acme"
}

# One document of the acme tenant.
resource "permitio_resource_instance" "quarterly_report" {
  key      = "quarterly-report"
  resource = permitio_resource.document.key
  tenant   = permitio_tenant.acme.key
  attributes = jsonencode({
    classified = true
  })
}
