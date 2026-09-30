terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A document in the document store"
  actions = {
    read = {
      name = "Read"
    }
    write = {
      name        = "Write"
      description = "Create or change a document"
    }
    delete = {
      name = "Delete"
    }
  }
  attributes = {
    owner = {
      type        = "string"
      description = "The key of the user who owns the document"
    }
    classified = {
      type = "bool"
    }
  }
}
