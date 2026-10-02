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

# Basic auth needs auth_secret.basic.
resource "permitio_proxy_config" "reports" {
  key            = "reports"
  name           = "Reports API"
  auth_mechanism = "Basic"
  auth_secret = {
    bearer = var.reports_api_token
  }
  mapping_rules = [
    {
      url         = "https://reports.example.com/documents"
      http_method = "get"
      resource    = "document"
      action      = "read"
    },
  ]
}
