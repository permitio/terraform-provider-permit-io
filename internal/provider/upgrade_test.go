package provider_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
	"github.com/permitio/terraform-provider-permit-io/internal/provider"
)

// The release an upgrade starts from, and where Terraform installs it from.
const (
	upgradeFromVersion    = "0.0.25"
	upgradeRegistryHost   = "registry.terraform.io"
	upgradeNamespace      = "permitio"
	upgradeProviderType   = "permit-io"
	upgradeProviderLocal  = "permitio"
	upgradeProviderSource = upgradeRegistryHost + "/" + upgradeNamespace + "/" +
		upgradeProviderType
)

// upgradeSumsPath is the SHA256SUMS file of the v0.0.25 release, byte for byte as
// published, so the release's detached signature still verifies against it. The
// test trusts only the checksums in this file, not the registry or the download.
const upgradeSumsPath = "testdata/upgrade/terraform-provider-permit-io_" +
	upgradeFromVersion + "_SHA256SUMS"

// upgradeExpectation is what this build must plan for one object that v0.0.25
// created from upgradeConfig.
type upgradeExpectation struct {
	// action is the planned action.
	action plancheck.ResourceActionType
	// changed lists, sorted, the attributes this build plans a known new value
	// for, or whose change forces the replacement. A computed attribute that is
	// only unknown until apply, such as a timestamp during an update, is left out.
	// It is empty for a no-op.
	changed []string
	// why is the documented reason for a planned change or an exclusion, with its
	// ticket. It is empty for a no-op.
	why string
	// excluded marks an object left out of upgradeConfig because v0.0.25 cannot
	// manage it against the mock; why names the bug.
	excluded bool
}

// noChange is the expectation for an object that upgrades with nothing to do.
var noChange = upgradeExpectation{action: plancheck.ResourceActionNoop}

// upgradeExpectations has an entry for every object in upgradeConfig, by address,
// and at least one for every resource type of the provider. A change that makes an
// upgrade from v0.0.25 plan something for an object replaces its noChange with the
// action, the changed attributes and the reason, as the upgrade guide documents
// them. An object v0.0.25 cannot manage against the mock is excluded with the bug
// in why, never skipped.
var upgradeExpectations = map[string]upgradeExpectation{
	"permitio_condition_set_rule.reviewers_read_drafts":           noChange,
	"permitio_group_resource_instance_role_assignment.developers": noChange,
	"permitio_proxy_config.billing":                               noChange,
	"permitio_proxy_config.ledger":                                noChange,
	"permitio_relation.parent":                                    noChange,
	"permitio_relation.tagged":                                    noChange,
	"permitio_resource.document":                                  noChange,
	"permitio_resource.folder":                                    noChange,
	"permitio_resource.tag":                                       noChange,
	"permitio_resource_instance.handbook":                         noChange,
	"permitio_resource_instance.memo":                             noChange,
	"permitio_resource_instance_role_assignment.bob":              noChange,
	"permitio_resource_set.all_documents":                         noChange,
	"permitio_resource_set.drafts":                                noChange,
	"permitio_role.editor":                                        noChange,
	"permitio_role.guest":                                         noChange,
	"permitio_role.owner":                                         noChange,
	"permitio_role.reader":                                        noChange,
	"permitio_role.viewer":                                        noChange,
	"permitio_role_assignment.alice":                              noChange,
	"permitio_role_derivation.owner_edits_documents":              noChange,
	"permitio_tenant.acme":                                        noChange,
	"permitio_tenant.sandbox":                                     noChange,
	"permitio_user_attribute.department":                          noChange,
	"permitio_user_set.reviewers":                                 noChange,
	"permitio_user_set.staff":                                     noChange,
}

// TestUpgradeFromV0025 applies upgradeConfig with the published v0.0.25 against the
// mock Permit API, then plans the same configuration with this build, which must
// plan what upgradeExpectations says for each object: nothing, until a change is
// documented. It then applies with this build, which must leave nothing to plan,
// and destroys with it. v0.0.25 comes from the Terraform registry and must match
// the checksum in upgradeSumsPath; when it cannot be fetched or does not match, the
// test fails.
func TestUpgradeFromV0025(t *testing.T) {
	checkUpgradeExpectations(t)
	t.Setenv("TF_CLI_CONFIG_FILE", upgradeMirrorConfig(t))
	// This build must serve the address v0.0.25 wrote into the state, so the
	// testing framework reattaches it as registry.terraform.io/permitio/permit-io,
	// also in the OpenTofu lanes, which set registry.opentofu.org for other tests.
	// Step 2 also excludes v0.0.25 by its version constraint, which Terraform
	// ignores for a reattached provider: if this build is not reattached at that
	// address, terraform init fails instead of installing v0.0.25 from the mirror
	// and comparing the release with itself.
	t.Setenv("TF_ACC_PROVIDER_HOST", upgradeRegistryHost)
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", upgradeNamespace)

	m := mockpermit.New(t, mockpermit.Resources, mockpermit.ResourceAttributes,
		mockpermit.Roles, mockpermit.ResourceRoles, mockpermit.ResourceRelations,
		mockpermit.ImplicitGrants, mockpermit.ConditionSets, mockpermit.ConditionSetRules,
		mockpermit.ProxyConfigs, mockpermit.Tenants, mockpermit.TenantList,
		mockpermit.ResourceInstances, mockpermit.RoleAssignments, mockpermit.GroupRoles)
	m.AddUser(`{"key": "alice", "email": "alice@example.com"}`)
	m.AddUser(`{"key": "bob", "email": "bob@example.com"}`)
	m.AddGroup("developers", "acme")

	resource.UnitTest(t, resource.TestCase{
		CheckDestroy: m.CheckStored("groups/developers", "users/alice", "users/bob"),
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					upgradeProviderLocal: {
						Source:            upgradeProviderSource,
						VersionConstraint: upgradeFromVersion,
					},
				},
				Config: upgradeConfig(upgradeFromVersion),
			},
			{
				ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
					upgradeProviderType: providerserver.NewProtocol6WithError(
						provider.New("test")()),
				},
				Config: upgradeConfig("!= " + upgradeFromVersion),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{expectUpgradePlan{}},
				},
			},
		},
	})
}

// checkUpgradeExpectations fails the test unless upgradeExpectations names an
// object of every resource type the provider serves and of no other type, and
// every entry that is not a no-op says why.
func checkUpgradeExpectations(t *testing.T) {
	t.Helper()
	snapshot, err := providerschema.Build(t.Context(), provider.New("test")())
	if err != nil {
		t.Fatalf("reading the provider's resource types: %v", err)
	}
	if len(snapshot.Resources) == 0 {
		t.Fatal("DID NOT RUN: the provider serves no resource types")
	}
	covered := map[string]bool{}
	for address, want := range upgradeExpectations {
		resourceType, _, _ := strings.Cut(address, ".")
		covered[resourceType] = true
		if _, ok := snapshot.Resources[resourceType]; !ok {
			t.Errorf("upgradeExpectations names %s, but the provider serves no %s",
				address, resourceType)
		}
		noOp := want.action == plancheck.ResourceActionNoop && len(want.changed) == 0 &&
			!want.excluded
		if !noOp && want.why == "" {
			t.Errorf("upgradeExpectations[%s] plans a change or excludes the object "+
				"without saying why", address)
		}
		if !slices.IsSorted(want.changed) {
			t.Errorf("upgradeExpectations[%s].changed is not sorted: %q", address,
				want.changed)
		}
	}
	for resourceType := range snapshot.Resources {
		if !covered[resourceType] {
			t.Errorf("upgradeExpectations has no %s; add one to upgradeConfig and "+
				"upgradeExpectations, or exclude it with the reason", resourceType)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

// upgradeConfig returns objects of every resource type, with the provider source
// and the version constraint. The first object of a type sets its optional
// attributes, so the state v0.0.25 writes holds them; a second object of some types
// leaves them out or takes another branch, so the state holds what v0.0.25 stores
// for an unset attribute. The two user role assignments use different users and
// roles, so the result does not depend on how role_assignment Read treats
// instance-level rows (PER-16599).
func upgradeConfig(version string) string {
	constraint := fmt.Sprintf("\n      version = %q", version)
	return fmt.Sprintf(`
terraform {
  required_providers {
    %s = {
      source  = %q%s
    }
  }
}
`, upgradeProviderLocal, upgradeProviderSource, constraint) + `
resource "permitio_resource" "folder" {
  key         = "folder"
  name        = "Folder"
  description = "A folder of documents"
  urn         = "prn:test:folder"
  actions = {
    list = { name = "List" }
  }
}

resource "permitio_resource" "document" {
  key         = "document"
  name        = "Document"
  description = "A text document"
  urn         = "prn:test:document"
  actions = {
    read  = { name = "Read" }
    write = { name = "Write", description = "Change the text" }
  }
  attributes = {
    owner = { type = "string", description = "Who owns the document" }
    pages = { type = "number" }
  }
}

resource "permitio_resource" "tag" {
  key  = "tag"
  name = "Tag"
  actions = {
    apply = { name = "Apply" }
  }
}

resource "permitio_role" "owner" {
  key         = "owner"
  name        = "Owner"
  description = "Owns a folder"
  resource    = permitio_resource.folder.key
  permissions = ["list"]
  extends     = []
}

resource "permitio_role" "reader" {
  key         = "reader"
  name        = "Reader"
  description = "Reads a document"
  resource    = permitio_resource.document.key
  permissions = ["read"]
  extends     = []
}

resource "permitio_role" "editor" {
  key         = "editor"
  name        = "Editor"
  description = "Edits a document"
  resource    = permitio_resource.document.key
  permissions = ["read", "write"]
  extends     = []
}

resource "permitio_role" "viewer" {
  key         = "viewer"
  name        = "Viewer"
  description = "Views everything"
  permissions = []
  extends     = []
}

resource "permitio_role" "guest" {
  key  = "guest"
  name = "Guest"
}

resource "permitio_relation" "parent" {
  key              = "parent"
  name             = "Parent folder"
  description      = "The folder that holds the document"
  subject_resource = permitio_resource.folder.key
  object_resource  = permitio_resource.document.key
}

resource "permitio_relation" "tagged" {
  key              = "tagged"
  name             = "Tagged with"
  subject_resource = permitio_resource.tag.key
  object_resource  = permitio_resource.document.key
}

resource "permitio_role_derivation" "owner_edits_documents" {
  resource    = permitio_resource.document.key
  to_role     = permitio_role.editor.key
  on_resource = permitio_resource.folder.key
  role        = permitio_role.owner.key
  linked_by   = permitio_relation.parent.key
}

resource "permitio_user_set" "reviewers" {
  key         = "reviewers"
  name        = "Reviewers"
  description = "Users who review documents"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.team" = { equals = "review" } }] }]
  })
}

resource "permitio_user_set" "staff" {
  key  = "staff"
  name = "Staff"
  conditions = jsonencode({
    allOf = [{ allOf = [{ "subject.employed" = { equals = true } }] }]
  })
}

resource "permitio_resource_set" "drafts" {
  key         = "drafts"
  name        = "Drafts"
  description = "Documents not yet published"
  resource    = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.status" = { equals = "draft" } }] }]
  })
}

resource "permitio_resource_set" "all_documents" {
  key      = "all_documents"
  name     = "All documents"
  resource = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [{ allOf = [{ "resource.kind" = { equals = "document" } }] }]
  })
}

resource "permitio_condition_set_rule" "reviewers_read_drafts" {
  user_set     = permitio_user_set.reviewers.key
  permission   = "${permitio_resource.document.key}:read"
  resource_set = permitio_resource_set.drafts.key
}

resource "permitio_proxy_config" "billing" {
  key            = "billing"
  name           = "Billing API"
  auth_mechanism = "Bearer"
  auth_secret = {
    bearer = "example-bearer-token"
  }
  mapping_rules = [
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "get"
      resource    = "invoice"
      action      = "read"
      priority    = 1
      headers     = { "x-tenant" = "required" }
    },
    {
      url         = "https://billing.example.com/v1/invoices"
      http_method = "post"
      resource    = "invoice"
    },
  ]
}

resource "permitio_proxy_config" "ledger" {
  key            = "ledger"
  name           = "Ledger API"
  auth_mechanism = "Basic"
  auth_secret = {
    basic = "example-user:example-password"
  }
  mapping_rules = [
    {
      url         = "https://ledger.example.com/v1/entries"
      http_method = "get"
      resource    = "entry"
    },
  ]
}

resource "permitio_user_attribute" "department" {
  key         = "department"
  type        = "string"
  description = "The department the user works in"
}

resource "permitio_tenant" "acme" {
  key         = "acme"
  name        = "Acme"
  description = "The acme tenant"
  attributes  = jsonencode({ tier = "gold" })
}

resource "permitio_tenant" "sandbox" {
  key  = "sandbox"
  name = "Sandbox"
}

resource "permitio_resource_instance" "handbook" {
  key        = "handbook"
  resource   = permitio_resource.document.key
  tenant     = permitio_tenant.acme.key
  attributes = jsonencode({ classification = "internal", pages = 12 })
}

resource "permitio_resource_instance" "memo" {
  key      = "memo"
  resource = permitio_resource.document.key
  tenant   = permitio_tenant.acme.key
}

resource "permitio_role_assignment" "alice" {
  user   = "alice"
  role   = permitio_role.viewer.key
  tenant = permitio_tenant.acme.key
}

resource "permitio_resource_instance_role_assignment" "bob" {
  user              = "bob"
  role              = permitio_role.reader.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.handbook.key
  tenant            = permitio_tenant.acme.key
}

resource "permitio_group_resource_instance_role_assignment" "developers" {
  group             = "developers"
  role              = permitio_role.editor.key
  resource          = permitio_resource.document.key
  resource_instance = permitio_resource_instance.handbook.key
  tenant            = permitio_tenant.acme.key
}
`
}

// planActions is the part of a planned change's actions that expectUpgradePlan
// reads. It stands in for tfjson.Actions so that this test does not make
// terraform-json a direct requirement in go.mod.
type planActions interface {
	NoOp() bool
	Update() bool
	DestroyBeforeCreate() bool
	CreateBeforeDestroy() bool
	Create() bool
	Delete() bool
	Read() bool
}

// actionType names planned actions the way plancheck.ExpectResourceAction does.
func actionType(actions planActions) plancheck.ResourceActionType {
	switch {
	case actions.NoOp():
		return plancheck.ResourceActionNoop
	case actions.Update():
		return plancheck.ResourceActionUpdate
	case actions.DestroyBeforeCreate():
		return plancheck.ResourceActionDestroyBeforeCreate
	case actions.CreateBeforeDestroy():
		return plancheck.ResourceActionCreateBeforeDestroy
	case actions.Create():
		return plancheck.ResourceActionCreate
	case actions.Delete():
		return plancheck.ResourceActionDestroy
	case actions.Read():
		return plancheck.ResourceActionRead
	}
	return plancheck.ResourceActionType(fmt.Sprint(actions))
}

// actionMatches reports whether the planned action is the expected one. Replace
// matches a replacement in either order.
func actionMatches(got, want plancheck.ResourceActionType) bool {
	if want == plancheck.ResourceActionReplace {
		return got == plancheck.ResourceActionDestroyBeforeCreate ||
			got == plancheck.ResourceActionCreateBeforeDestroy
	}
	return got == want
}

// expectUpgradePlan fails unless the plan does what upgradeExpectations says for
// every object, and has every object the table does not exclude. Its error lists
// each changed attribute of an unexpected change, so a failure reads as a plan
// diff.
type expectUpgradePlan struct{}

func (expectUpgradePlan) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest,
	resp *plancheck.CheckPlanResponse,
) {
	var errs []error
	planned := map[string]bool{}
	for _, rc := range req.Plan.ResourceChanges {
		planned[rc.Address] = true
		want, ok := upgradeExpectations[rc.Address]
		if !ok {
			errs = append(errs, fmt.Errorf("%s is not in upgradeExpectations; add it with "+
				"noChange or with its documented upgrade plan", rc.Address))
			continue
		}
		if want.excluded {
			errs = append(errs, fmt.Errorf("upgradeExpectations excludes %s (%s), but "+
				"upgradeConfig has it", rc.Address, want.why))
			continue
		}
		change := plannedChange{
			before:          asObject(rc.Change.Before),
			after:           asObject(rc.Change.After),
			afterUnknown:    asObject(rc.Change.AfterUnknown),
			beforeSensitive: asObject(rc.Change.BeforeSensitive),
			afterSensitive:  asObject(rc.Change.AfterSensitive),
			replacePaths:    rc.Change.ReplacePaths,
		}
		got := actionType(rc.Change.Actions)
		changed, diff := change.diff()
		if actionMatches(got, want.action) && slices.Equal(changed, want.changed) {
			continue
		}
		expected := string(want.action)
		if want.why != "" {
			expected += " (" + want.why + ")"
		}
		errs = append(errs, fmt.Errorf("%s: this build plans %s, changing %q; "+
			"upgradeExpectations wants %s, changing %q\n%s",
			rc.Address, got, changed, expected, want.changed, diff))
	}
	for address, want := range upgradeExpectations {
		if !want.excluded && !planned[address] {
			errs = append(errs, fmt.Errorf("upgradeConfig has no %s; add it, or exclude it "+
				"in upgradeExpectations with the reason", address))
		}
	}
	resp.Error = errors.Join(errs...)
}

// plannedChange is the prior state and the plan of one object, each an object of
// attribute values as terraform show -json writes them.
type plannedChange struct {
	before, after, afterUnknown     map[string]any
	beforeSensitive, afterSensitive map[string]any
	replacePaths                    []any
}

// diff returns the sorted names of the attributes this build plans a known new
// value for, or whose change forces a replacement, and one line for every attribute
// whose planned value differs from the prior state, including one that is only
// unknown until apply, such as a computed timestamp during an update.
func (c plannedChange) diff() ([]string, string) {
	names := map[string]bool{}
	for _, values := range []map[string]any{c.before, c.after, c.afterUnknown} {
		for name := range values {
			names[name] = true
		}
	}
	var changed []string
	var lines strings.Builder
	for _, name := range slices.Sorted(maps.Keys(names)) {
		unknown := hasTrue(c.afterUnknown[name])
		if !unknown && reflect.DeepEqual(c.before[name], c.after[name]) {
			continue
		}
		forcesReplacement := c.forcesReplacement(name)
		if forcesReplacement || knownChange(c.before[name], c.after[name],
			c.afterUnknown[name]) {
			changed = append(changed, name)
		}
		after := render(c.after[name], c.afterSensitive[name])
		if c.afterUnknown[name] == true {
			after = "(known after apply)"
		} else if unknown {
			after += " (partly known after apply)"
		}
		fmt.Fprintf(&lines, "  ~ %s: %s -> %s", name, render(c.before[name],
			c.beforeSensitive[name]), after)
		if forcesReplacement {
			lines.WriteString("  # forces replacement")
		}
		lines.WriteString("\n")
	}
	return changed, lines.String()
}

// knownChange reports whether a planned value differs from the prior one in a part
// that is known before apply. unknown is the value's after_unknown marks.
func knownChange(before, after, unknown any) bool {
	if unknown == true {
		return false
	}
	switch after := after.(type) {
	case map[string]any:
		beforeObject, ok := before.(map[string]any)
		if !ok {
			return true
		}
		unknownObject, _ := unknown.(map[string]any)
		for key := range beforeObject {
			if _, planned := after[key]; !planned && unknownObject[key] != true {
				return true
			}
		}
		for key, value := range after {
			if knownChange(beforeObject[key], value, unknownObject[key]) {
				return true
			}
		}
		return false
	case []any:
		beforeList, ok := before.([]any)
		if !ok || len(beforeList) != len(after) {
			return true
		}
		unknownList, _ := unknown.([]any)
		for i, value := range after {
			var mark any
			if i < len(unknownList) {
				mark = unknownList[i]
			}
			if knownChange(beforeList[i], value, mark) {
				return true
			}
		}
		return false
	}
	return !reflect.DeepEqual(before, after)
}

// forcesReplacement reports whether a replace path starts at the attribute.
func (c plannedChange) forcesReplacement(name string) bool {
	for _, replacePath := range c.replacePaths {
		steps, ok := replacePath.([]any)
		if ok && len(steps) > 0 && steps[0] == name {
			return true
		}
	}
	return false
}

// asObject returns an object value from terraform show -json, or nil for any
// other value, such as the missing prior state of an object being created.
func asObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

// hasTrue reports whether a value from after_unknown or a *_sensitive object marks
// the attribute or any part of it.
func hasTrue(marks any) bool {
	switch marks := marks.(type) {
	case bool:
		return marks
	case map[string]any:
		for _, mark := range marks {
			if hasTrue(mark) {
				return true
			}
		}
	case []any:
		if slices.ContainsFunc(marks, hasTrue) {
			return true
		}
	}
	return false
}

// render writes an attribute value as JSON, or hides it when it is sensitive.
func render(value, sensitive any) string {
	if hasTrue(sensitive) {
		return "(sensitive value)"
	}
	if value == nil {
		return "null"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(encoded)
}

// upgradeMirrorConfig downloads this platform's build of v0.0.25 from the Terraform
// registry into a filesystem mirror, after checking it against upgradeSumsPath, and
// returns a Terraform CLI configuration that installs the provider only from that
// mirror. It fails the test when the build cannot be fetched or does not match.
func upgradeMirrorConfig(t *testing.T) string {
	t.Helper()
	platform := runtime.GOOS + "_" + runtime.GOARCH
	archive, err := fetchUpgradeArchive(t.Context(), platform)
	if err != nil {
		t.Fatalf("could not fetch v%s of the provider for %s: %v", upgradeFromVersion,
			platform, err)
	}
	mirror := t.TempDir()
	packageDir := filepath.Join(mirror, upgradeRegistryHost, upgradeNamespace,
		upgradeProviderType)
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("creating the provider mirror: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, archive.name), archive.data,
		0o644); err != nil {
		t.Fatalf("writing v%s to the provider mirror: %v", upgradeFromVersion, err)
	}
	config := filepath.Join(t.TempDir(), "terraformrc")
	content := fmt.Sprintf(`provider_installation {
  filesystem_mirror {
    path    = %q
    include = [%q]
  }
  direct {
    exclude = [%q]
  }
}
`, filepath.ToSlash(mirror), upgradeProviderSource, upgradeProviderSource)
	if err := os.WriteFile(config, []byte(content), 0o644); err != nil {
		t.Fatalf("writing the Terraform CLI configuration: %v", err)
	}
	return config
}

// upgradeArchive is a release zip of the provider.
type upgradeArchive struct {
	name string
	data []byte
}

// fetchUpgradeArchive asks the registry where the v0.0.25 zip for the platform is,
// downloads it, and returns it only if its SHA-256 is the one in upgradeSumsPath.
func fetchUpgradeArchive(ctx context.Context, platform string) (upgradeArchive, error) {
	name := "terraform-provider-" + upgradeProviderType + "_" + upgradeFromVersion + "_" +
		platform + ".zip"
	pinned, err := pinnedChecksum(name)
	if err != nil {
		return upgradeArchive{}, err
	}
	goos, goarch, _ := strings.Cut(platform, "_")
	downloadURL := fmt.Sprintf("https://%s/v1/providers/%s/%s/%s/download/%s/%s",
		upgradeRegistryHost, upgradeNamespace, upgradeProviderType, upgradeFromVersion,
		goos, goarch)
	body, err := httpGet(ctx, downloadURL, 1<<20)
	if err != nil {
		return upgradeArchive{}, err
	}
	var download struct {
		Filename    string `json:"filename"`
		DownloadURL string `json:"download_url"`
		Shasum      string `json:"shasum"`
	}
	if err := json.Unmarshal(body, &download); err != nil {
		return upgradeArchive{}, fmt.Errorf("reading the registry's answer from %s: %w",
			downloadURL, err)
	}
	if download.Filename != name || download.Shasum != pinned {
		return upgradeArchive{}, fmt.Errorf("the registry lists %s with SHA-256 %s, "+
			"but %s pins %s with %s", download.Filename, download.Shasum,
			upgradeSumsPath, name, pinned)
	}
	data, err := httpGet(ctx, download.DownloadURL, 256<<20)
	if err != nil {
		return upgradeArchive{}, err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != pinned {
		return upgradeArchive{}, fmt.Errorf("%s from %s has SHA-256 %s, but %s pins %s",
			name, download.DownloadURL, got, upgradeSumsPath, pinned)
	}
	return upgradeArchive{name: name, data: data}, nil
}

// pinnedChecksum returns the SHA-256 that upgradeSumsPath lists for the file.
func pinnedChecksum(name string) (string, error) {
	sums, err := os.Open(upgradeSumsPath)
	if err != nil {
		return "", fmt.Errorf("reading the pinned checksums: %w", err)
	}
	defer sums.Close()
	scanner := bufio.NewScanner(sums)
	for scanner.Scan() {
		sum, file, ok := strings.Cut(scanner.Text(), "  ")
		if ok && file == name {
			return sum, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading %s: %w", upgradeSumsPath, err)
	}
	return "", fmt.Errorf("%s lists no %s; v%s was not published for this platform",
		upgradeSumsPath, name, upgradeFromVersion)
}

// httpGet returns the body of a 200 response to a GET of the URL, trying up to
// three times, and fails a body longer than limit bytes.
func httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		body, err := httpGetOnce(ctx, client, url, limit)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("GET %s failed 3 times, last with: %w", url, lastErr)
}

func httpGetOnce(ctx context.Context, client *http.Client, url string, limit int64) (
	[]byte, error,
) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("the body is longer than %d bytes", limit)
	}
	return body, nil
}
