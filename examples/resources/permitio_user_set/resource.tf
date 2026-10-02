terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

resource "permitio_user_attribute" "department" {
  key  = "department"
  type = "string"
}

# The users whose department attribute is "finance".
resource "permitio_user_set" "finance" {
  key         = "finance"
  name        = "Finance department"
  description = "Users in the finance department"
  conditions = jsonencode({
    allOf = [
      { allOf = [{ "subject.department" = { equals = "finance" } }] },
    ]
  })
  depends_on = [permitio_user_attribute.department]
}
