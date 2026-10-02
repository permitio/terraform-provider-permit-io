terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# A tenant has a name, not a display_name.
resource "permitio_tenant" "main" {
  key          = "main"
  name         = "Main"
  display_name = "Main"
}
