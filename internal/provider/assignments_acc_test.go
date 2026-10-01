package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
)

// testAccAssignmentObjects are the keys of the objects the assignment acceptance
// tests assign roles on: a document resource with a reader role, a tenant, and a
// document of the tenant.
type testAccAssignmentObjects struct {
	resource, role, tenant, instance string
}

func newTestAccAssignmentObjects(testID string) testAccAssignmentObjects {
	return testAccAssignmentObjects{
		resource: testID + "-doc",
		role:     testID + "-reader",
		tenant:   testID + "-acme",
		instance: testID + "-readme",
	}
}

// config returns the configuration of the objects followed by extra.
func (o testAccAssignmentObjects) config(extra string) string {
	return providerConfig + fmt.Sprintf(`
		resource "permitio_resource" "doc" {
			key  = %q
			name = "Document"
			actions = {
				read = { name = "Read" }
			}
		}
		resource "permitio_role" "reader" {
			key         = %q
			name        = "Reader"
			resource    = permitio_resource.doc.key
			permissions = ["read"]
		}
		resource "permitio_tenant" "acme" {
			key  = %q
			name = "Acme"
		}
		resource "permitio_resource_instance" "readme" {
			key      = %q
			resource = permitio_resource.doc.key
			tenant   = permitio_tenant.acme.key
		}`, o.resource, o.role, o.tenant, o.instance) + extra
}

// onInstance names the assignment of the reader role to user on the document.
func (o testAccAssignmentObjects) onInstance(user string) testAccAssignmentKeys {
	return testAccAssignmentKeys{
		user: user, role: o.role, tenant: o.tenant, instance: o.resource + ":" + o.instance,
	}
}

// TestAccRoleAssignment checks against the Permit API that permitio_role_assignment
// tracks the user's tenant-level assignment of a top-level role when the user also
// has the document's resource role with the same key on a document of the tenant,
// assigned through the API before Terraform makes the tenant-level one, so that the
// list of the user's assignments of that role key in the tenant can hold both. The
// resource keeps the tenant-level assignment's ID through a refresh that plans
// nothing and an import by user:role:tenant. Deleted outside Terraform, the
// assignment is planned for creation again rather than read from the one on the
// document. Removed from the configuration, it is unassigned and the one on the
// document is left.
func TestAccRoleAssignment(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	objects := newTestAccAssignmentObjects(testID)
	user := testID + "-user"
	const address = "permitio_role_assignment.reader"
	topLevelRole := fmt.Sprintf(`
		resource "permitio_role" "top_level_reader" {
			key         = %q
			name        = "Top-level reader"
			permissions = []
		}`, objects.role)
	withoutAssignment := objects.config(topLevelRole)
	withAssignment := objects.config(topLevelRole + fmt.Sprintf(`
		resource "permitio_role_assignment" "reader" {
			user   = %q
			role   = permitio_role.top_level_reader.key
			tenant = permitio_tenant.acme.key
		}`, user))
	inTenant := testAccAssignmentKeys{user: user, role: objects.role, tenant: objects.tenant}
	onInstance := objects.onInstance(user)
	tracksTenantLevel := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(address, "user", user),
		resource.TestCheckResourceAttr(address, "role", objects.role),
		resource.TestCheckResourceAttr(address, "tenant", objects.tenant),
		testAccCheckAssignmentInState(address, inTenant),
		testAccCheckAssignment(onInstance, true),
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccCreateUser(t, user) },
				Config:    withoutAssignment,
			},
			{
				PreConfig: func() {
					_, err := testAccClient(t).Api.Users.AssignResourceRole(t.Context(),
						onInstance.user, onInstance.role, onInstance.tenant, onInstance.instance)
					if err != nil {
						t.Fatalf("making %s outside Terraform: %v", onInstance, err)
					}
				},
				Config: withAssignment,
				Check:  tracksTenantLevel,
			},
			{
				Config: withAssignment,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: tracksTenantLevel,
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     strings.Join([]string{user, objects.role, objects.tenant}, ":"),
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					_, err := testAccClient(t).Api.Users.UnassignRole(t.Context(),
						inTenant.user, inTenant.role, inTenant.tenant)
					if err != nil {
						t.Fatalf("deleting %s outside Terraform: %v", inTenant, err)
					}
				},
				Config: withAssignment,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate),
					},
				},
				Check: tracksTenantLevel,
			},
			{
				Config: withoutAssignment,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckAssignment(inTenant, false),
					testAccCheckAssignment(onInstance, true),
				),
			},
		},
	})
}

// TestAccResourceInstanceRoleAssignment checks against the Permit API that
// permitio_resource_instance_role_assignment assigns a resource role to a user on a
// resource instance, keeps it through a refresh that plans nothing and an import by
// user:role:resource:resource_instance:tenant, and unassigns it when it leaves the
// configuration while the user, role and instance still exist.
func TestAccResourceInstanceRoleAssignment(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	objects := newTestAccAssignmentObjects(testID)
	user := testID + "-user"
	const address = "permitio_resource_instance_role_assignment.reader"
	withAssignment := objects.config(fmt.Sprintf(`
		resource "permitio_resource_instance_role_assignment" "reader" {
			user              = %q
			role              = permitio_role.reader.key
			resource          = permitio_resource.doc.key
			resource_instance = permitio_resource_instance.readme.key
			tenant            = permitio_tenant.acme.key
		}`, user))
	onInstance := objects.onInstance(user)
	tracked := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(address, "user", user),
		resource.TestCheckResourceAttr(address, "role", objects.role),
		resource.TestCheckResourceAttr(address, "resource", objects.resource),
		resource.TestCheckResourceAttr(address, "resource_instance", objects.instance),
		resource.TestCheckResourceAttr(address, "tenant", objects.tenant),
		testAccCheckAssignmentInState(address, onInstance),
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccCreateUser(t, user) },
				Config:    withAssignment,
				Check:     tracked,
			},
			{
				Config: withAssignment,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: tracked,
			},
			{
				ResourceName: address,
				ImportState:  true,
				ImportStateId: strings.Join([]string{
					user, objects.role, objects.resource, objects.instance, objects.tenant,
				}, ":"),
				ImportStateVerify: true,
			},
			{
				Config: objects.config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy),
					},
				},
				Check: testAccCheckAssignment(onInstance, false),
			},
		},
	})
}

// TestAccGroupResourceInstanceRoleAssignment checks against the Permit API that
// permitio_group_resource_instance_role_assignment assigns a resource role to a group
// on a resource instance, keeps it through a refresh that plans nothing and an
// import by group:role:resource:resource_instance:tenant, and removes it when it
// leaves the configuration while the group, role and instance still exist. The
// provider has no group resource, so the test makes the group through the API, in
// the tenant Terraform made, before Terraform assigns it the role. The group has the
// default group resource type, group, which the test neither makes nor removes. The
// last step deletes the group while its tenant still exists, when the spec promises
// a 204, because it does not say what deleting a group answers once Terraform has
// destroyed the group's tenant.
func TestAccGroupResourceInstanceRoleAssignment(t *testing.T) {
	testID := acctest.RandomWithPrefix(testAccKeyPrefix)
	objects := newTestAccAssignmentObjects(testID)
	group := testID + "-team"
	var deleteGroup func()
	const address = "permitio_group_resource_instance_role_assignment.team"
	withAssignment := objects.config(fmt.Sprintf(`
		resource "permitio_group_resource_instance_role_assignment" "team" {
			group             = %q
			role              = permitio_role.reader.key
			resource          = permitio_resource.doc.key
			resource_instance = permitio_resource_instance.readme.key
			tenant            = permitio_tenant.acme.key
		}`, group))
	groupRole := testAccGroupRoleKeys{
		group: group, role: objects.role, resource: objects.resource, instance: objects.instance,
	}
	tracked := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(address, "id", group),
		resource.TestCheckResourceAttr(address, "group", group),
		resource.TestCheckResourceAttr(address, "role", objects.role),
		resource.TestCheckResourceAttr(address, "resource", objects.resource),
		resource.TestCheckResourceAttr(address, "resource_instance", objects.instance),
		resource.TestCheckResourceAttr(address, "tenant", objects.tenant),
		testAccCheckGroupRole(groupRole, true),
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy,
		Steps: []resource.TestStep{
			{
				Config: objects.config(""),
			},
			{
				PreConfig: func() { deleteGroup = testAccCreateGroup(t, group, objects.tenant) },
				Config:    withAssignment,
				Check:     tracked,
			},
			{
				Config: withAssignment,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: tracked,
			},
			{
				ResourceName: address,
				ImportState:  true,
				ImportStateId: strings.Join([]string{
					group, objects.role, objects.resource, objects.instance, objects.tenant,
				}, ":"),
				ImportStateVerify: true,
			},
			{
				Config: objects.config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy),
					},
				},
				Check: testAccCheckGroupRole(groupRole, false),
			},
			{
				PreConfig: func() { deleteGroup() },
				Config:    objects.config(""),
			},
		},
	})
}

// testAccClient builds the SDK client of testAccPermitClient, or stops the test.
func testAccClient(t *testing.T) *permit.Client {
	t.Helper()
	client, err := testAccPermitClient()
	if err != nil {
		t.Fatalf("building the Permit client: %v", err)
	}
	return client
}

// testAccCreateUser creates a user with this key through the API, since the
// provider has no user resource. Deleting the user when the test ends, even when it
// fails, also removes any role assignment of the user the test leaves behind.
func testAccCreateUser(t *testing.T, key string) {
	t.Helper()
	client := testAccClient(t)
	t.Cleanup(func() {
		// The test's context is canceled before cleanup runs.
		err := client.Api.Users.Delete(context.WithoutCancel(t.Context()), key)
		if err != nil && !common.IsNotFoundErr(err) {
			t.Errorf("deleting user %s: %v", key, err)
		}
	})
	if _, err := client.Api.Users.Create(t.Context(), *models.NewUserCreate(key)); err != nil {
		t.Fatalf("creating user %s: %v", key, err)
	}
}

// testAccCreateGroup creates a group with this key in the tenant through the API,
// since the provider has no group resource, and returns a function that deletes it.
// Unless that function has deleted the group, it is deleted when the test ends, even
// when the test fails.
func testAccCreateGroup(t *testing.T, key, tenant string) (deleteGroup func()) {
	t.Helper()
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		// The test's context is canceled before cleanup runs.
		if err := testAccDeleteGroup(context.WithoutCancel(t.Context()), key); err != nil {
			t.Errorf("deleting group %s: %v", key, err)
		}
	})
	_, err := testAccGroupsRequest(t.Context(), http.MethodPost, "",
		map[string]string{"group_instance_key": key, "group_tenant": tenant})
	if err != nil {
		t.Fatalf("creating group %s in tenant %s: %v", key, tenant, err)
	}
	return func() {
		t.Helper()
		if err := testAccDeleteGroup(t.Context(), key); err != nil {
			t.Fatalf("deleting group %s: %v", key, err)
		}
		deleted = true
	}
}

// testAccDeleteGroup deletes the group with this key through the API. A group that
// is already gone is not an error.
func testAccDeleteGroup(ctx context.Context, key string) error {
	_, err := testAccGroupsRequest(ctx, http.MethodDelete, "/"+url.PathEscape(key), nil)
	if err != nil && !common.IsNotFoundErr(err) {
		return err
	}
	return nil
}

// testAccCheckAssignment checks through the API that the assignment with these keys
// exists when want is true, and that it does not when want is false.
func testAccCheckAssignment(keys testAccAssignmentKeys, want bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := testAccPermitClient()
		if err != nil {
			return fmt.Errorf("building the client to look for %s: %w", keys, err)
		}
		_, found, err := testAccFindAssignment(context.Background(), client, keys)
		if err != nil {
			return err
		}
		if found != want {
			return fmt.Errorf("%s exists in Permit: %t, want %t", keys, found, want)
		}
		return nil
	}
}

// testAccCheckAssignmentInState checks through the API that the assignment with
// these keys exists and that the resource at address holds its ID, so the resource
// tracks that assignment and no other.
func testAccCheckAssignmentInState(
	address string, keys testAccAssignmentKeys,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("%s is not in the state", address)
		}
		client, err := testAccPermitClient()
		if err != nil {
			return fmt.Errorf("building the client to look for %s: %w", keys, err)
		}
		assignment, found, err := testAccFindAssignment(context.Background(), client, keys)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%s is not in Permit", keys)
		}
		if got := rs.Primary.Attributes["id"]; got != assignment.Id {
			return fmt.Errorf("%s has the id %q, want %q, the ID of %s", address, got,
				assignment.Id, keys)
		}
		return nil
	}
}

// testAccCheckGroupRole checks through the API that the group has the role on the
// resource instance when want is true, and that it does not when want is false.
func testAccCheckGroupRole(keys testAccGroupRoleKeys, want bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		found, err := testAccGroupRoleExists(context.Background(), keys)
		if err != nil {
			return err
		}
		if found != want {
			return fmt.Errorf("group %s has role %s on %s:%s in Permit: %t, want %t",
				keys.group, keys.role, keys.resource, keys.instance, found, want)
		}
		return nil
	}
}
