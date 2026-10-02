terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# attributes is a JSON object in a string, such as jsonencode returns.
resource "permitio_tenant" "main" {
  key        = "main"
  name       = "Main"
  attributes = "region=eu"
}
