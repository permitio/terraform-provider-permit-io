package relations_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

var providerFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"permitio": providerserver.NewProtocol6WithError(provider.New("test")()),
}

const (
	fileRelationsPath = "/v2/schema/" + mockpermit.ProjectID + "/" +
		mockpermit.EnvironmentID + "/resources/file/relations"
	relationBody = `{
		"key": "parent",
		"name": "Parent folder",
		"description": "The folder that holds the file",
		"subject_resource": "folder"
	}`
)

// TestRelationCreateDestroy runs permitio_relation through Terraform against the
// mock Permit API and checks the exact body the provider sends. Changing a
// relation's name or description fails today, so the second step changes the
// object resource instead and checks that the relation is left alone.
func TestRelationCreateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRelations)
	const address = "permitio_relation.parent"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: relationConfig(`read = { name = "Read" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(address, "id"),
					resource.TestCheckResourceAttr(address, "key", "parent"),
					resource.TestCheckResourceAttr(address, "name", "Parent folder"),
					resource.TestCheckResourceAttr(address, "description",
						"The folder that holds the file"),
					resource.TestCheckResourceAttr(address, "subject_resource", "folder"),
					resource.TestCheckResourceAttrPair(address, "subject_resource_id",
						"permitio_resource.folder", "id"),
					resource.TestCheckResourceAttr(address, "object_resource", "file"),
					resource.TestCheckResourceAttrPair(address, "object_resource_id",
						"permitio_resource.file", "id"),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
					m.CheckRequests(http.MethodPost, fileRelationsPath, relationBody),
				),
			},
			{
				Config: relationConfig(`read  = { name = "Read" }
    write = { name = "Write" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("permitio_resource.file",
							plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "object_resource", "file"),
					m.CheckRequests(http.MethodPost, fileRelationsPath, relationBody),
					m.CheckRequests(http.MethodDelete, fileRelationsPath+"/parent"),
				),
			},
		},
	})

	m.AssertAllRoutesHit()
}

// relationConfig returns the folder and file resources and the relation that puts
// files in folders, with fileActions as the body of the file's actions.
func relationConfig(fileActions string) string {
	return fmt.Sprintf(`
resource "permitio_resource" "folder" {
  key         = "folder"
  name        = "Folder"
  description = "A folder of files"
  urn         = "prn:test:folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_resource" "file" {
  key         = "file"
  name        = "File"
  description = "A file in a folder"
  urn         = "prn:test:file"
  actions = {
    %s
  }
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  description      = "The folder that holds the file"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}
`, fileActions)
}
