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
  attributes = {}
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
  attributes = {}
}

resource "permitio_relation" "parent" {
  key = "parent"
  name = "parent of"
  subject_resource = permitio_resource.folder.key
  object_resource = permitio_resource.file.key
}
