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

# The documents whose classified attribute is true.
resource "permitio_resource_set" "classified_documents" {
  key      = "classified_documents"
  name     = "Classified documents"
  resource = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [
      { allOf = [{ "resource.classified" = { equals = true } }] },
    ]
  })
}
