variable "reports_api_token" {
  description = "Token the Permit proxy sends to the reports API."
  type        = string
  sensitive   = true
}

variable "reports_base_url" {
  description = "Base URL of the reports API, such as https://reports.example.com."
  type        = string
}

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
      resource    = permitio_resource.document.key
      action      = "read"
    },
    {
      url         = "https://reports.example.com/documents/{document_id}"
      http_method = "put"
      resource    = permitio_resource.document.key
      action      = "write"
      priority    = 1
      headers = {
        "x-request-source" = "permit-proxy"
      }
    },
    {
      url         = "^https://reports\\.example\\.com/documents/[0-9]+/pages$"
      url_type    = "regex"
      http_method = "get"
      resource    = permitio_resource.document.key
      action      = "read"
    },
    # Two urls built from a variable with the same http_method: validate does not
    # know their values yet, so it must not take them for the same rule.
    {
      url         = "${var.reports_base_url}/archive"
      http_method = "get"
      resource    = permitio_resource.document.key
      action      = "read"
    },
    {
      url         = "${var.reports_base_url}/exports"
      http_method = "get"
      resource    = permitio_resource.document.key
      action      = "read"
    },
  ]
}
