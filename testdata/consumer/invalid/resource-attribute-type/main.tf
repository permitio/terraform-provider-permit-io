terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# text is not an attribute type; string is.
resource "permitio_resource" "document" {
  key  = "document"
  name = "Document"
  actions = {
    read = { name = "Read" }
  }
  attributes = {
    title = { type = "text" }
  }
}
