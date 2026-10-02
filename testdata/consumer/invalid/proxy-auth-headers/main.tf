terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

variable "reports_api_key" {
  type      = string
  sensitive = true
}

# The provider does not support Headers authentication yet.
resource "permitio_proxy_config" "reports" {
  key            = "reports"
  name           = "Reports API"
  auth_mechanism = "Headers"
  auth_secret = {
    headers = {
      "x-api-key" = var.reports_api_key
    }
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
