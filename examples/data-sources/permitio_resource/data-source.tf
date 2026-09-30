terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

data "permitio_resource" "document" {
  key = "document"
}

# A role that may take every action the document resource has.
resource "permitio_role" "document_admin" {
  key  = "document_admin"
  name = "Document admin"
  permissions = [
    for action in keys(data.permitio_resource.document.actions) :
    "${data.permitio_resource.document.key}:${action}"
  ]
}
