terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# A role needs a key.
resource "permitio_role" "viewer" {
  name        = "Viewer"
  permissions = ["document:read"]
}
