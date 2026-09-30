terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# updated_at is set by Permit when the role changes, so it cannot be configured.
resource "permitio_role" "main" {
  key        = "main"
  name       = "Main"
  updated_at = "2026-01-01T00:00:00Z"
}
