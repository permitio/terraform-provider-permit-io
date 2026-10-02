resource "permitio_tenant" "main" {
  key         = "main"
  name        = "Main"
  description = "The default tenant"
  attributes  = jsonencode({ region = "eu" })
}

resource "permitio_resource_instance" "handbook" {
  key        = "handbook"
  resource   = permitio_resource.document.key
  tenant     = permitio_tenant.main.key
  attributes = jsonencode({ title = "Handbook", published = true })
}

resource "permitio_resource_instance" "policies" {
  key      = "policies"
  resource = permitio_resource.folder.key
  tenant   = permitio_tenant.main.key
}

resource "permitio_role_assignment" "alice_admin" {
  user   = data.permitio_user.alice.key
  role   = data.permitio_role.admin.key
  tenant = permitio_tenant.main.key
}

resource "permitio_resource_instance_role_assignment" "alice_edits_handbook" {
  user              = data.permitio_user.alice.key
  role              = permitio_role.document_editor.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.handbook.key
  tenant            = permitio_tenant.main.key
}

resource "permitio_group_resource_instance_role_assignment" "writers_own_policies" {
  group             = "writers"
  role              = permitio_role.folder_owner.key
  resource          = permitio_resource.folder.key
  resource_instance = permitio_resource_instance.policies.key
  tenant            = permitio_tenant.main.key
}
