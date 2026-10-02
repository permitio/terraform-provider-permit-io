terraform {
  required_providers {
    permitio = {
      source  = "permitio/permit-io"
      version = "~> 1.0"
    }
  }
}

variable "permitio_api_key" {
  description = "An environment-level Permit.io API key."
  type        = string
  sensitive   = true
}

# api_key can be left out when the PERMITIO_API_KEY environment variable is set.
provider "permitio" {
  api_key = var.permitio_api_key
}
