terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# A user is looked up by its key.
data "permitio_user" "alice" {
}
