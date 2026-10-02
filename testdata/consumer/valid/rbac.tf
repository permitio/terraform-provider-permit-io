resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A document in the knowledge base"
  actions = {
    read  = { name = "Read" }
    write = { name = "Write" }
    delete = {
      name        = "Delete"
      description = "Delete the document"
    }
  }
  attributes = {
    title = {
      type        = "string"
      description = "The document's title"
    }
    published = { type = "bool" }
  }
}

resource "permitio_resource" "folder" {
  key  = "folder"
  name = "Folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Reads every document and lists every folder"
  permissions = ["document:read", "folder:list"]
  depends_on  = [permitio_resource.document, permitio_resource.folder]
}

resource "permitio_role" "writer" {
  key         = "writer"
  name        = "Writer"
  permissions = ["document:write"]
  extends     = [permitio_role.viewer.key]
  depends_on  = [permitio_resource.document]
}

resource "permitio_user_attribute" "department" {
  key         = "department"
  type        = "string"
  description = "The department the user works in"
}
