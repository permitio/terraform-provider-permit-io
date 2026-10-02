terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

resource "permitio_tenant" "acme" {
  key         = "acme"
  name        = "Acme"
  description = "The Acme customer account"
  attributes = jsonencode({
    tier   = "enterprise"
    region = "eu"
  })
}
