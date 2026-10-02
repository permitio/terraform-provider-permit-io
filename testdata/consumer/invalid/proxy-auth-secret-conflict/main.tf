terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# auth_secret holds one kind of secret. The values are known, not variables,
# because the check skips unknown values.
resource "permitio_proxy_config" "reports" {
  key            = "reports"
  name           = "Reports API"
  auth_mechanism = "Bearer"
  auth_secret = {
    bearer = "example-token"
    basic  = "example-user:example-password"
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
