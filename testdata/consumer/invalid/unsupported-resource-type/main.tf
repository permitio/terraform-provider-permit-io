terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# The resource type is permitio_tenant.
resource "permitio_tenants" "main" {
  key  = "main"
  name = "Main"
}
