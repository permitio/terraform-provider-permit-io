terraform {
  required_providers {
    permitio = {
      source = "registry.terraform.io/permitio/permit-io"
    }
  }
}

provider "permitio" {
  api_key = "SET ENV - PERMITIO_API_KEY"
}

resource "permitio_resource" "file" {
  key  = "document"
  name = "document"
  actions = {
    "create" = {
      "name" = "create"
    }
    "read" = {
      "name" = "read"
    }
    "update" = {
      "name" = "update"
    }
    "delete" = {
      "name" = "delete"
    }
  }
}

resource "permitio_resource" "folder" {
  key  = "folder"
  name = "folder"
  actions = {
    "create" = {
      "name" = "create"
    }
    "read" = {
      "name" = "read"
    }
    "update" = {
      "name" = "update"
    }
    "modify" = {
      "name" = "modify"
    }
  }
}