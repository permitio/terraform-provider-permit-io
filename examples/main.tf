terraform {
  required_providers {
    permitio = {
      source = "registry.terraform.io/permitio/permit-io"
    }
  }
}

provider "permitio" {
  api_key = "SET ENV - PERMITIO_API_KEY"
}


resource "permitio_resource" "document" {
  key         = "document"
  name        = "document"
  description = "a new document"
  actions = {
    "read" = {
      "name" = "read"
    }
    "write" = {
      "name" = "write"
    }
    "delete" = {
      "name"        = "write"
      "description" = "delete a document"
    }
  }
}

resource "permitio_role" "writer" {
  key         = "writer"
  name        = "writer"
  description = "a new admin"
  permissions = ["document:read", "document:write", "document:delete"]
  depends_on = [
    "permitio_resource.document"
  ]
}
resource "permitio_role" "admin" {
  key         = "admin"
  name        = "admin"
  description = "a new admin"
  permissions = ["document:read", "document:write"]
  extends     = []
  depends_on = [
    "permitio_resource.document",
    "permitio_role.writer"
  ]
}

output "my_resource" {
  value = permitio_role.admin
}