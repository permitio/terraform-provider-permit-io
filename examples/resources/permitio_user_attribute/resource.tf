terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

resource "permitio_user_attribute" "department" {
  key         = "department"
  type        = "string"
  description = "The department the user works in"
}
