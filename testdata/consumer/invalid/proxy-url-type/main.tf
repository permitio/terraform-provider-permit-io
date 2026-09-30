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

# url_type is regex, or omitted for a URL; the API has no glob match.
resource "permitio_proxy_config" "reports" {
  key            = "reports"
  name           = "Reports API"
  auth_mechanism = "Bearer"
  auth_secret = {
    bearer = var.reports_api_token
  }
  mapping_rules = [
    {
      url         = "https://reports.example.com/documents/*"
      url_type    = "glob"
      http_method = "get"
      resource    = "document"
      action      = "read"
    },
  ]
}
