terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# A relation names its resources by key; a resource ID never matches what
# Permit returns, so a UUID there fails validation.
resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  subject_resource = "4f6c1e2a-7b3d-4c5e-9f8a-1b2c3d4e5f60"
  object_resource  = "file"
}
