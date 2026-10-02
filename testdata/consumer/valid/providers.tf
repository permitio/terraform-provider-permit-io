terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

variable "permit_api_key" {
  description = "API key of the Permit.io environment to manage."
  type        = string
  sensitive   = true
}

provider "permitio" {
  api_url = "https://api.permit.io"
  api_key = var.permit_api_key
  timeout = 30
}
