terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

data "permitio_user_attribute" "department" {
  key = "department"
}

output "department_attribute_type" {
  value = data.permitio_user_attribute.department.type
}
