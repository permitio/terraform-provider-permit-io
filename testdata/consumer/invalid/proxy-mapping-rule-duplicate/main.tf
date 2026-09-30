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

# The API identifies a mapping rule by its url and http_method, so two rules
# cannot share both.
resource "permitio_proxy_config" "reports" {
  key            = "reports"
  name           = "Reports API"
  auth_mechanism = "Bearer"
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
    {
      url         = "https://reports.example.com/documents"
      http_method = "get"
      resource    = "document"
      action      = "list"
    },
  ]
}
