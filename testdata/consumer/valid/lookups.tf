# Objects managed outside this configuration, looked up by key.

data "permitio_user" "alice" {
  key = "alice"
}

data "permitio_role" "admin" {
  key = "admin"
}

data "permitio_resource" "invoice" {
  key = "invoice"
}

data "permitio_condition_set" "staff" {
  key = "staff"
}

data "permitio_user_attribute" "email" {
  key = "email"
}

output "alice_email" {
  value = data.permitio_user.alice.email
}

output "invoice_actions" {
  value = keys(data.permitio_resource.invoice.actions)
}

output "staff_conditions" {
  value = data.permitio_condition_set.staff.conditions
}

output "email_attribute_type" {
  value = data.permitio_user_attribute.email.type
}
