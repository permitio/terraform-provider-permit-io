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
					// Admin Role tests
					resource.TestCheckResourceAttr("permitio_role.admin", "key", "admin"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.#", "3"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.0", "document:read"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.1", "document:write"),
					resource.TestCheckResourceAttr("permitio_role.admin", "permissions.2", "document:delete"),
				),
			},
		},
	})
}
