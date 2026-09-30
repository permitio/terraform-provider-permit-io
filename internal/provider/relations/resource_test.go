package relations_test

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
// mock Permit API and checks the exact body the provider sends to create it.
// Changing a relation's name or description fails today, so there is no update
// step.
func TestRelationCreateDestroy(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceRelations)
	const address = "permitio_relation.parent"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckEmpty,
		Steps: []resource.TestStep{
			{
				Config: relationConfig,
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
		},
	})

	m.AssertRoutesHit(mockpermit.ResourceRelations)
}

// relationConfig is the folder and file resources and the relation that puts files
// in folders.
const relationConfig = `
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
    read = { name = "Read" }
  }
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  description      = "The folder that holds the file"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.file.key
}
`
