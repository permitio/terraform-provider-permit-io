terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

data "permitio_user" "jane" {
  key = "jane@example.com"
}

output "jane_email" {
  value = data.permitio_user.jane.email
}

# attributes is a JSON object, or null when the user has no attributes.
output "jane_department" {
  value = try(jsondecode(data.permitio_user.jane.attributes).department, null)
}
