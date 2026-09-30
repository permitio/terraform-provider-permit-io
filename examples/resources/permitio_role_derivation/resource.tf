terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

resource "permitio_resource" "folder" {
  key  = "folder"
  name = "Folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_resource" "file" {
  key  = "file"
  name = "File"
  actions = {
    read  = { name = "Read" }
    write = { name = "Write" }
  }
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}

resource "permitio_role" "folder_manager" {
  key         = "manager"
  name        = "Manager"
  resource    = permitio_resource.folder.key
  permissions = ["list"]
}

resource "permitio_role" "file_editor" {
  key         = "editor"
  name        = "Editor"
  resource    = permitio_resource.file.key
  permissions = ["read", "write"]
}

# The manager of a folder is an editor of every file in it.
resource "permitio_role_derivation" "folder_managers_edit_files" {
  role        = permitio_role.folder_manager.key
  on_resource = permitio_resource.folder.key
  to_role     = permitio_role.file_editor.key
  resource    = permitio_resource.file.key
  linked_by   = permitio_relation.parent.key
}
