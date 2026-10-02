terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# permissions is a set of strings, not one string.
resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  permissions = "document:read"
}
