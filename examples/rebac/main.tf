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
  key     = "file"
  name    = "file"
  actions = {
    "create" = {
      "name" = "Create"
    }
    "read" = {
      "name" = "Read"
    }
    "update" = {
      "name" = "Update"
    }
    "delete" = {
      "name" = "Delete"
    }
  }
  attributes = {}
}

resource "permitio_resource" "folder" {
  key     = "folder"
  name    = "folder"
  actions = {
    "create" = {
      "name" = "Create"
    }
    "list" = {
      "name" = "List"
    }
    "modify" = {
      "name" = "Modify"
    }
    "delete" = {
      "name" = "Delete"
    }
  }
  attributes = {}
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "parent of"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}

resource "permitio_role" "fileAdmin" {
  key         = "admin"
  name        = "Administrator"
  description = "Administrator access to files"
  permissions = ["read", "create", "update", "delete"]
  extends     = []
  resource    = permitio_resource.file.key
  depends_on  = [
    permitio_resource.file,
  ]
}

resource "permitio_role" "folderAdmin" {
  key         = "admin"
  name        = "Administrator"
  description = "Administrator access to folders"
  permissions = ["create", "list", "modify", "delete"]
  extends     = []
  resource    = permitio_resource.folder.key
  depends_on  = [
    permitio_resource.folder,
  ]
}

resource "permitio_role_derivation" "folderFileAdmin" {
  resource    = permitio_resource.file.key
  role        = permitio_role.fileAdmin.key
  on_resource = permitio_resource.folder.key
  to_role     = permitio_role.folderAdmin.key
  linked_by   = permitio_relation.parent.key
}