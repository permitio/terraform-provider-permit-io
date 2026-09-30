variable "billing_api_token" {
  description = "The token the Permit Proxy sends to the billing API."
  type        = string
  sensitive   = true
}

resource "permitio_proxy_config" "billing" {
  key            = "billing"
  name           = "Billing API"
  auth_mechanism = "Bearer"
  auth_secret = {
    bearer = var.billing_api_token
  }
  mapping_rules = [
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "get"
      resource    = "invoice"
      action      = "read"
    },
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "post"
      resource    = "invoice"
      action      = "create"
    },
  ]
}
