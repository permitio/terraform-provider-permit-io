terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# text is not an attribute type; string is.
resource "permitio_user_attribute" "department" {
  key  = "department"
  type = "text"
}
