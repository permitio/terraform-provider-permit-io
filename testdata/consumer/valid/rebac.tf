resource "permitio_role" "folder_owner" {
  key         = "owner"
  name        = "Owner"
  resource    = permitio_resource.folder.key
  permissions = ["list"]
}

resource "permitio_role" "document_editor" {
  key         = "editor"
  name        = "Editor"
  description = "Reads and writes the document"
  resource    = permitio_resource.document.key
  permissions = ["read", "write"]
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent"
  description      = "The folder that holds the document"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.document.key
}

# Owners of a folder edit every document in it.
resource "permitio_role_derivation" "owner_edits_documents" {
  role        = permitio_role.folder_owner.key
  on_resource = permitio_resource.folder.key
  to_role     = permitio_role.document_editor.key
  resource    = permitio_resource.document.key
  linked_by   = permitio_relation.parent.key
}
