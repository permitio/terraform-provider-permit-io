terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# Permit creates a resource instance only in a tenant.
resource "permitio_resource_instance" "handbook" {
  key      = "handbook"
  resource = "document"
}
