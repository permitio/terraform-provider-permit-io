terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

variable "reports_api_token" {
  type      = string
  sensitive = true
}

# Every mapping rule needs a url.
resource "permitio_proxy_config" "reports" {
  key            = "reports"
  name           = "Reports API"
  auth_mechanism = "Bearer"
  auth_secret = {
    bearer = var.reports_api_token
  }
  mapping_rules = [
    {
      http_method = "get"
      resource    = "document"
      action      = "read"
    },
  ]
}
