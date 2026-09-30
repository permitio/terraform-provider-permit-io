package users_test

import (
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

// TestUserDataSourceRead reads a user the mock Permit API serves through the
// permitio_user data source and checks every attribute it exports.
func TestUserDataSourceRead(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Users)
	aliceID := m.AddUser(`{"key": "alice", "email": "alice@example.com", "first_name": "Alice",
		"last_name": "Doe", "attributes": {"team": "docs", "level": 3}}`)
	const address = "data.permitio_user.alice"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories,
		CheckDestroy:             m.CheckStored("users/alice"),
		Steps: []resource.TestStep{
			{
				Config: `
data "permitio_user" "alice" {
  key = "alice"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", aliceID),
					resource.TestCheckResourceAttr(address, "key", "alice"),
					resource.TestCheckResourceAttr(address, "email", "alice@example.com"),
					resource.TestCheckResourceAttr(address, "first_name", "Alice"),
					resource.TestCheckResourceAttr(address, "last_name", "Doe"),
					resource.TestCheckResourceAttr(address, "attributes",
						`{"level":3,"team":"docs"}`),
					resource.TestCheckResourceAttr(address, "organization_id",
						mockpermit.OrganizationID),
					resource.TestCheckResourceAttr(address, "project_id", mockpermit.ProjectID),
					resource.TestCheckResourceAttr(address, "environment_id",
						mockpermit.EnvironmentID),
				),
			},
		},
	})

	m.AssertRoutesHit(mockpermit.Users)
}
