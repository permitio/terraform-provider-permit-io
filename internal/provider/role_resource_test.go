package provider

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	permitConfig "github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

func TestResources(t *testing.T) {
	testID := fmt.Sprintf("test-%d-%d", time.Now().Unix(), rand.Intn(10000))
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

// TestRoleDerivation covers issue #30 with distinct role keys on the two resources:
// folder admins (`role` on `on_resource`) get the file admin role (`to_role` on
// `resource`). It checks the grant through the API, so it fails if the provider sends
// role and to_role the other way round, and it checks that a derivation deleted
// outside Terraform is planned for re-creation instead of failing the refresh.
func TestRoleDerivation(t *testing.T) {
	testID := fmt.Sprintf("test-%d-%d", time.Now().Unix(), rand.Intn(10000))
	fileKey := "file-" + testID
	folderKey := "folder-" + testID
	parentKey := "parent-" + testID
	fileAdminKey := "admin-" + testID
	folderAdminKey := "folder-admin-" + testID
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
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  checks,
			},
			{
				PreConfig: func() {
					// ImplicitGrants.Delete's parameter names are swapped in the SDK: the
					// first argument fills the URL's resource slot, the second its role slot.
					err := testAccPermitClient().Api.ImplicitGrants.Delete(
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

// testAccPermitClient builds an SDK client from the same environment variables the
// provider reads, so tests can change Permit state outside Terraform.
func testAccPermitClient() *permit.Client {
	apiURL := os.Getenv("PERMITIO_API_URL")
	if apiURL == "" {
		apiURL = DefaultApiUrl
	}
	return permit.NewPermit(
		permitConfig.NewConfigBuilder(os.Getenv("PERMITIO_API_KEY")).WithApiUrl(apiURL).Build(),
	)
}

// testAccCheckRoleGrantedTo checks through the API that roleKey on resourceKey is
// derived from want.Role on want.OnResource via want.LinkedByRelation.
func testAccCheckRoleGrantedTo(
	t *testing.T, resourceKey, roleKey string, want models.DerivedRoleRuleRead,
) resource.TestCheckFunc {
	return func(*terraform.State) error {
		role, err := testAccPermitClient().Api.ResourceRoles.Get(t.Context(), resourceKey, roleKey)
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
