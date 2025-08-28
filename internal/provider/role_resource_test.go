package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestResources(t *testing.T) {
	testID := fmt.Sprintf("test-%d", time.Now().Unix())
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_resource" "document" {
						key		 = "document-%s"
						name	 = "document-%s"
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
					}
					resource "permitio_role" "admin" {
						  key         = "admin-%s"
						  name        = "admin"	
						  description = "a new admin"	
						  permissions = ["document-%s:read"]
							depends_on = [
							"permitio_resource.document"
						  ]	
					  }
					resource "permitio_role" "writer" {
							  key         = "writer-%s"
							  name        = "writer"
							  description = "a new writer"
							  permissions = [
								"document-%s:write"
							  ]
							  extends = [
								"admin-%s"
							  ]
							  depends_on = [
								"permitio_role.admin",
								"permitio_resource.document"
							  ]		
							}`, testID, testID, testID, testID, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Document Resource tests
					resource.TestCheckResourceAttr("permitio_resource.document", "key", fmt.Sprintf("document-%s", testID)),
					resource.TestCheckResourceAttr("permitio_resource.document", "name", fmt.Sprintf("document-%s", testID)),
					resource.TestCheckResourceAttr("permitio_resource.document", "description", "a new document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "actions.read.name", "read"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.type", "time"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.description", "creation time of the document"),
					// Admin Role tests
					resource.TestCheckResourceAttr("permitio_role.admin", "key", fmt.Sprintf("admin-%s", testID)),
					resource.TestCheckResourceAttr("permitio_role.admin", "name", "admin"),
					resource.TestCheckResourceAttr("permitio_role.admin", "description", "a new admin"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.#", "1"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.0", fmt.Sprintf("document-%s:read", testID)),
					// Writer Role tests
					resource.TestCheckResourceAttr("permitio_role.writer", "key", fmt.Sprintf("writer-%s", testID)),
					resource.TestCheckResourceAttr("permitio_role.writer", "name", "writer"),
					resource.TestCheckResourceAttr("permitio_role.writer", "description", "a new writer"),
					resource.TestCheckResourceAttr("permitio_role.writer", "permissions.#", "1"),
					resource.TestCheckResourceAttr("permitio_role.writer", "permissions.0", fmt.Sprintf("document-%s:write", testID)),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_resource" "document" {
						key		 = "document-%s"
						name	 = "document-%s"
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
					}
					resource "permitio_role" "admin" {
							  key         = "admin-%s"
							  name        = "admin"	
							  description = "a new admin"	
							  permissions = ["document-%s:read", "document-%s:write", "document-%s:delete"]
								depends_on = [
								"permitio_resource.document"
							  ]
							  }`, testID, testID, testID, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Document Resource tests
					resource.TestCheckResourceAttr("permitio_resource.document", "key", fmt.Sprintf("document-%s", testID)),
					resource.TestCheckResourceAttr("permitio_resource.document", "actions.delete.name", "delete"),
					resource.TestCheckResourceAttr("permitio_resource.document", "actions.delete.description", "delete a document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.type", "number"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.created_at.description", "creation time of the document"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.content.type", "string"),
					resource.TestCheckResourceAttr("permitio_resource.document", "attributes.content.description", "the content of the document"),
					// Admin Role tests
					resource.TestCheckResourceAttr("permitio_role.admin", "key", fmt.Sprintf("admin-%s", testID)),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.#", "3"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_proxy_config" "foaz" {
				  key            = "foaz-%s"
				  name           = "Boaz"
				  auth_mechanism = "Basic"
				  auth_secret = {
					basic = "hello:world"
				  }
				  mapping_rules = [
					{
					  url         = "https://example.com/documents"
					  http_method = "post"
					  resource    = "document-%s"
					  action      = "read"
					},
					{
					  url         = "https://example.com/documents/{project_id}"
					  http_method = "delete"
					  resource    = "document-%s"
					  action      = "delete"
					}
				  ]
				}`, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Proxy Config tests
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "key", fmt.Sprintf("foaz-%s", testID)),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "name", "Boaz"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_mechanism", "Basic"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_secret.basic", "hello:world"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "mapping_rules.#", "2"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_proxy_config" "foaz" {
					  key            = "foaz-%s"
					  name           = "Boaz"
					  auth_mechanism = "Basic"
					  auth_secret = {
						basic = "hello:world"
					  }
					  mapping_rules = [
						{
						  url         = "https://example.com/documents"
						  http_method = "post"
						  resource    = "document-%s"
						  action      = "read"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "delete"
						  resource    = "document-%s"
						  action      = "delete"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "get"
						  resource    = "document-%s"
						  action      = "read"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "put"
						  resource    = "document-%s"
						  action      = "update"
						  headers = {
							"x-update-id" : "foaz"
						  }
						}
					  ]
				}`, testID, testID, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Proxy Config tests
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "key", fmt.Sprintf("foaz-%s", testID)),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "name", "Boaz"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_mechanism", "Basic"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "auth_secret.basic", "hello:world"),
					resource.TestCheckResourceAttr("permitio_proxy_config.foaz", "mapping_rules.#", "4"),
				),
			},
			{Config: providerConfig + fmt.Sprintf(`resource "permitio_resource" "file" {
				key  = "file-%s"
				name = "file-%s"
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
				key  = "folder-%s"
				name = "folder-%s"
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
				key              = "parent-%s"
				name             = "parent of"
				subject_resource = permitio_resource.folder.key
				object_resource  = permitio_resource.file.key
			}
			
				resource "permitio_role" "fileAdmin" {
				key         = "admin-%s"
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
				key         = "admin-%s"
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
			}`, testID, testID, testID, testID, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(),
			},
		},
	})
}
