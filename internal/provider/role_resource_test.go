package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestResources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig +
					`resource "permitio_resource" "document" {
						key		 = "document"
						name	 = "document"
						description = "a new document"
						actions = {
								"read" = {
									"name" = "read"
								}
								"write" = {		
									"name" = "write"
								}
						}
						attributes = {
							"created_at" = {
								"description" = "creation time of the document"
							  	"type"        = "time"
							}
						}
					}` + "\n" +
					`resource "permitio_role" "admin" {
						  key         = "admin"
						  name        = "admin"	
						  description = "a new admin"	
						  permissions = ["document:read"]
							depends_on = [
							"permitio_resource.document"
						  ]	
					  }` + "\n" +
					`resource "permitio_role" "writer" {
							  key         = "writer"
							  name        = "writer"
							  description = "a new writer"
							  permissions = [
								"document:write"
							  ]
							  extends = [
								"admin"
							  ]
							  depends_on = [
								"permitio_role.admin",
								"permitio_resource.document"
							  ]		
							}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Document Resource tests
					resource.TestCheckResourceAttr("permitio_resource.document", "key", "document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "name", "document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "description", "a new document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "actions.read.name", "read"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.type", "time"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.description", "creation time of the document"),
					// Admin Role tests
					resource.TestCheckResourceAttr("permitio_role.admin", "key", "admin"),
					resource.TestCheckResourceAttr("permitio_role.admin", "name", "admin"),
					resource.TestCheckResourceAttr("permitio_role.admin", "description", "a new admin"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.#", "1"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.0", "document:read"),
					// Writer Role tests
					resource.TestCheckResourceAttr("permitio_role.writer", "key", "writer"),
					resource.TestCheckResourceAttr("permitio_role.writer", "name", "writer"),
					resource.TestCheckResourceAttr("permitio_role.writer", "description", "a new writer"),
					resource.TestCheckResourceAttr("permitio_role.writer", "permissions.#", "1"),
					resource.TestCheckResourceAttr("permitio_role.writer", "permissions.0", "document:write"),
				),
			},
			{
				Config: providerConfig +
					`resource "permitio_resource" "document" {
						key		 = "document"
						name	 = "document"
						description = "a new document"
						actions = {
							"read" = {
								"name" = "read"
							}
							"write" = {		
								"name" = "write"
							}
							"delete" = {		
								"name" = "delete"
								"description" = "delete a document"
							}
						}
						attributes = {
							"created_at" = {
								"description" = "creation time of the document"
							  	"type"        = "number"
							}
							"content" = {
								"description" = "the content of the document"	
								"type"        = "string"
							}
						}
					}` + "\n" +
					`resource "permitio_role" "admin" {
							  key         = "admin"
							  name        = "admin"	
							  description = "a new admin"	
							  permissions = ["document:read", "document:write", "document:delete"]
								depends_on = [
								"permitio_resource.document"
							  ]
							  }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Document Resource tests
					resource.TestCheckResourceAttr("permitio_resource.document", "key", "document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "actions.delete.name", "delete"),
					resource.TestCheckResourceAttr("permitio_resource.document", "actions.delete.description", "delete a document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.type", "number"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.description", "creation time of the document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.content.type", "string"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.content.description", "the content of the document"),
					// Admin Role tests
					resource.TestCheckResourceAttr("permitio_role.admin", "key", "admin"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.#", "3"),
				),
			},
			{
				Config: providerConfig +
					`resource "permitio_proxy_config" "foaz" {
				  key            = "foaz"
				  name           = "Boaz"
				  auth_mechanism = "Basic"
				  auth_secret = {
					basic = "hello:world"
				  }
				  mapping_rules = [
					{
					  url         = "https://example.com/documents"
					  http_method = "post"
					  resource    = "document"
					  action      = "read"
					},
					{
					  url         = "https://example.com/documents/{project_id}"
					  http_method = "delete"
					  resource    = "document"
					  action      = "delete"
					}
				  ]
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Proxy Config tests
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "key", "foaz"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "name", "Boaz"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_mechanism", "Basic"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_secret.basic", "hello:world"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "mapping_rules.#", "2"),
				),
			},
			{
				Config: providerConfig +
					`resource "permitio_proxy_config" "foaz" {
					  key            = "foaz"
					  name           = "Boaz"
					  auth_mechanism = "Basic"
					  auth_secret = {
						basic = "hello:world"
					  }
					  mapping_rules = [
						{
						  url         = "https://example.com/documents"
						  http_method = "post"
						  resource    = "document"
						  action      = "read"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "get"
						  resource    = "document"
						  action      = "read"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "put"
						  resource    = "document"
						  action      = "update"
						  headers = {
							"x-update-id" : "foaz"
						  }
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "delete"
						  resource    = "document"
						  action      = "delete"
						}
					  ]
					  depends_on = [
						permitio_resource.document
					  ]
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Proxy Config tests
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "key", "foaz"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "name", "Boaz"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_mechanism", "Basic"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_secret.basic", "hello:world"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "mapping_rules.#", "4"),
				),
			},
			{Config: providerConfig +
				`resource "permitio_resource" "file" {
				key  = "file"
				name = "file"
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
			attributes = {
				"created_at" = {
					"description" = "creation time of the document"
					"type"        = "time"
					}
				}
			}
				resource "permitio_resource" "folder" {
				key  = "folder"
				name = "folder"
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
			attributes = {
				"created_at" = {
					"description" = "creation time of the document"
					"type"        = "time"
					}
				}
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
				depends_on = [
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
				depends_on = [
				permitio_resource.folder,
			]
			}
			
				resource "permitio_role_derivation" "folderFileAdmin" {
				resource    = permitio_resource.file.key
				to_role        = permitio_role.fileAdmin.key
				on_resource = permitio_resource.folder.key
				role     = permitio_role.folderAdmin.key
				linked_by   = permitio_relation.parent.key
			}`,
				Check: resource.ComposeAggregateTestCheckFunc(),
			},
		},
	})
}
