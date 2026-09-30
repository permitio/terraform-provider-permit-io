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
    read = { name = "Read" }
  }
}

# "folder is parent of file": the relation is defined on the file resource, the
# object, and points to the folder resource, the subject.
resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent"
  description      = "The folder that holds the file"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}
