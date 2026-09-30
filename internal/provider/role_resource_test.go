package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/permit-golang/pkg/models"
)

func TestAccResources(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	documentKey := testID + "-document"
	const (
		doc    = "permitio_resource.document"
		admin  = "permitio_role.admin"
		writer = "permitio_role.writer"
		proxy  = "permitio_proxy_config.foaz"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_resource" "document" {
						key		 = "%s-document"
						name	 = "%s-document"
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
						  key         = "%s-admin"
						  name        = "admin"	
						  description = "a new admin"	
						  permissions = ["%s-document:read"]
							depends_on = [
							"permitio_resource.document"
						  ]	
					  }
					resource "permitio_role" "writer" {
							  key         = "%s-writer"
							  name        = "writer"
							  description = "a new writer"
							  permissions = [
								"%s-document:write"
							  ]
							  extends = [
								"%s-admin"
							  ]
							  depends_on = [
								"permitio_role.admin",
								"permitio_resource.document"
							  ]		
							}`, testID, testID, testID, testID, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Document Resource tests
					resource.TestCheckResourceAttr(doc, "key", documentKey),
					resource.TestCheckResourceAttr(doc, "name", documentKey),
					resource.TestCheckResourceAttr(doc, "description", "a new document"),
					resource.TestCheckResourceAttr(doc, "actions.read.name", "read"),
					resource.TestCheckResourceAttr(doc, "attributes.created_at.type", "time"),
					resource.TestCheckResourceAttr(doc, "attributes.created_at.description",
						"creation time of the document"),
					// Admin Role tests
					resource.TestCheckResourceAttr(admin, "key", testID+"-admin"),
					resource.TestCheckResourceAttr(admin, "name", "admin"),
					resource.TestCheckResourceAttr(admin, "description", "a new admin"),
					resource.TestCheckResourceAttr(admin, "permissions.#", "1"),
					resource.TestCheckResourceAttr(admin, "permissions.0", documentKey+":read"),
					// Writer Role tests
					resource.TestCheckResourceAttr(writer, "key", testID+"-writer"),
					resource.TestCheckResourceAttr(writer, "name", "writer"),
					resource.TestCheckResourceAttr(writer, "description", "a new writer"),
					resource.TestCheckResourceAttr(writer, "permissions.#", "1"),
					resource.TestCheckResourceAttr(writer, "permissions.0", documentKey+":write"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_resource" "document" {
						key		 = "%s-document"
						name	 = "%s-document"
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
							  key         = "%s-admin"
							  name        = "admin"	
							  description = "a new admin"	
							  permissions = [
								"%s-document:read",
								"%s-document:write",
								"%s-document:delete",
							  ]
								depends_on = [
								"permitio_resource.document"
							  ]
							  }`, testID, testID, testID, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Document Resource tests
					resource.TestCheckResourceAttr(doc, "key", documentKey),
					resource.TestCheckResourceAttr(doc, "actions.delete.name", "delete"),
					resource.TestCheckResourceAttr(doc, "actions.delete.description",
						"delete a document"),
					resource.TestCheckResourceAttr(doc, "attributes.created_at.type", "number"),
					resource.TestCheckResourceAttr(doc, "attributes.created_at.description",
						"creation time of the document"),
					resource.TestCheckResourceAttr(doc, "attributes.content.type", "string"),
					resource.TestCheckResourceAttr(doc, "attributes.content.description",
						"the content of the document"),
					// Admin Role tests
					resource.TestCheckResourceAttr(admin, "key", testID+"-admin"),
					resource.TestCheckResourceAttr(admin, "permissions.#", "3"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_proxy_config" "foaz" {
				  key            = "%s-foaz"
				  name           = "Boaz"
				  auth_mechanism = "Basic"
				  auth_secret = {
					basic = "hello:world"
				  }
				  mapping_rules = [
					{
					  url         = "https://example.com/documents"
					  http_method = "post"
					  resource    = "%s-document"
					  action      = "read"
					},
					{
					  url         = "https://example.com/documents/{project_id}"
					  http_method = "delete"
					  resource    = "%s-document"
					  action      = "delete"
					}
				  ]
				}`, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Proxy Config tests
					resource.TestCheckResourceAttr(proxy, "key", testID+"-foaz"),
					resource.TestCheckResourceAttr(proxy, "name", "Boaz"),
					resource.TestCheckResourceAttr(proxy, "auth_mechanism", "Basic"),
					resource.TestCheckResourceAttr(proxy, "auth_secret.basic", "hello:world"),
					resource.TestCheckResourceAttr(proxy, "mapping_rules.#", "2"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`resource "permitio_proxy_config" "foaz" {
					  key            = "%s-foaz"
					  name           = "Boaz"
					  auth_mechanism = "Basic"
					  auth_secret = {
						basic = "hello:world"
					  }
					  mapping_rules = [
						{
						  url         = "https://example.com/documents"
						  http_method = "post"
						  resource    = "%s-document"
						  action      = "read"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "delete"
						  resource    = "%s-document"
						  action      = "delete"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "get"
						  resource    = "%s-document"
						  action      = "read"
						},
						{
						  url         = "https://example.com/documents/{project_id}"
						  http_method = "put"
						  resource    = "%s-document"
						  action      = "update"
						  headers = {
							"x-update-id" : "foaz"
						  }
						}
					  ]
				}`, testID, testID, testID, testID, testID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Proxy Config tests
					resource.TestCheckResourceAttr(proxy, "key", testID+"-foaz"),
					resource.TestCheckResourceAttr(proxy, "name", "Boaz"),
					resource.TestCheckResourceAttr(proxy, "auth_mechanism", "Basic"),
					resource.TestCheckResourceAttr(proxy, "auth_secret.basic", "hello:world"),
					resource.TestCheckResourceAttr(proxy, "mapping_rules.#", "4"),
				),
			},
			{Config: providerConfig + fmt.Sprintf(`resource "permitio_resource" "file" {
				key  = "%s-file"
				name = "%s-file"
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
				key  = "%s-folder"
				name = "%s-folder"
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
				key              = "%s-parent"
				name             = "parent of"
				subject_resource = permitio_resource.folder.key
				object_resource  = permitio_resource.file.key
			}
			
				resource "permitio_role" "fileAdmin" {
				key         = "%s-admin"
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
				key         = "%s-admin"
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

// TestAccRoleDerivation covers derivations with distinct role keys on the two resources:
// folder admins (`role` on `on_resource`) get the file admin role (`to_role` on
// `resource`). It checks the grant through the API, so it fails if the provider sends
// role and to_role the other way round, and it checks that a derivation deleted
// outside Terraform is planned for re-creation instead of failing the refresh.
func TestAccRoleDerivation(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	fileKey := testID + "-file"
	folderKey := testID + "-folder"
	parentKey := testID + "-parent"
	fileAdminKey := testID + "-admin"
	folderAdminKey := testID + "-folder-admin"
	const address = "permitio_role_derivation.folderFileAdmin"

	config := providerConfig + fmt.Sprintf(`
		resource "permitio_resource" "file" {
			key  = %[1]q
			name = %[1]q
			actions = {
				"read" = { "name" = "Read" }
			}
			attributes = {}
		}
		resource "permitio_resource" "folder" {
			key  = %[2]q
			name = %[2]q
			actions = {
				"read" = { "name" = "Read" }
			}
			attributes = {}
		}
		resource "permitio_relation" "parent" {
			key              = %[3]q
			name             = "parent of"
			subject_resource = permitio_resource.folder.key
			object_resource  = permitio_resource.file.key
		}
		resource "permitio_role" "fileAdmin" {
			key         = %[4]q
			name        = "File Administrator"
			permissions = ["read"]
			resource    = permitio_resource.file.key
		}
		resource "permitio_role" "folderAdmin" {
			key         = %[5]q
			name        = "Folder Administrator"
			permissions = ["read"]
			resource    = permitio_resource.folder.key
		}
		resource "permitio_role_derivation" "folderFileAdmin" {
			role        = permitio_role.folderAdmin.key
			on_resource = permitio_resource.folder.key
			to_role     = permitio_role.fileAdmin.key
			resource    = permitio_resource.file.key
			linked_by   = permitio_relation.parent.key
		}`, fileKey, folderKey, parentKey, fileAdminKey, folderAdminKey)

	grant := models.DerivedRoleRuleRead{
		Role:             folderAdminKey,
		OnResource:       folderKey,
		LinkedByRelation: parentKey,
	}
	checks := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(address, "role", folderAdminKey),
		resource.TestCheckResourceAttr(address, "on_resource", folderKey),
		resource.TestCheckResourceAttr(address, "to_role", fileAdminKey),
		resource.TestCheckResourceAttr(address, "resource", fileKey),
		resource.TestCheckResourceAttr(address, "linked_by", parentKey),
		testAccCheckRoleGrantedTo(t, fileKey, fileAdminKey, grant),
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  checks,
			},
			{
				PreConfig: func() {
					client, err := testAccPermitClient()
					if err != nil {
						t.Fatalf("building the client to delete the role derivation: %v", err)
					}
					// ImplicitGrants.Delete's parameter names are swapped in the SDK: the
					// first argument fills the URL's resource slot, the second its role slot.
					err = client.Api.ImplicitGrants.Delete(
						t.Context(), fileKey, fileAdminKey, models.DerivedRoleRuleDelete{
							Role:             grant.Role,
							OnResource:       grant.OnResource,
							LinkedByRelation: grant.LinkedByRelation,
						})
					if err != nil {
						t.Fatalf("deleting role derivation outside Terraform: %v", err)
					}
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate),
					},
				},
				Check: checks,
			},
		},
	})
}

// testAccCheckRoleGrantedTo checks through the API that roleKey on resourceKey is
// derived from want.Role on want.OnResource via want.LinkedByRelation.
func testAccCheckRoleGrantedTo(
	t *testing.T, resourceKey, roleKey string, want models.DerivedRoleRuleRead,
) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := testAccPermitClient()
		if err != nil {
			return fmt.Errorf("building the client to check role %s/%s: %w",
				resourceKey, roleKey, err)
		}
		role, err := client.Api.ResourceRoles.Get(t.Context(), resourceKey, roleKey)
		if err != nil {
			return fmt.Errorf("getting role %s/%s: %w", resourceKey, roleKey, err)
		}
		if role.GrantedTo != nil {
			for _, got := range role.GrantedTo.UsersWithRole {
				if got.Role == want.Role && got.OnResource == want.OnResource &&
					got.LinkedByRelation == want.LinkedByRelation {
					return nil
				}
			}
		}
		return fmt.Errorf("role %s/%s is not derived from %s on %s via %s",
			resourceKey, roleKey, want.Role, want.OnResource, want.LinkedByRelation)
	}
}
