package provider

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/providerschema"
)

// undescribedAttributes lists, sorted, the attributes that have no description
// yet, by Terraform address: "permitio_role.key" for a resource and
// "data.permitio_role.key" for a data source, with the dotted path of a nested
// attribute. The list only shrinks: when an attribute gets a description, remove
// it here. A new attribute needs a description instead of an entry.
var undescribedAttributes = []string{
	"data.permitio_condition_set.conditions",
	"data.permitio_condition_set.description",
	"data.permitio_condition_set.environment_id",
	"data.permitio_condition_set.id",
	"data.permitio_condition_set.key",
	"data.permitio_condition_set.name",
	"data.permitio_condition_set.organization_id",
	"data.permitio_condition_set.project_id",
	"data.permitio_condition_set.resource",
	"data.permitio_condition_set.type",
	"data.permitio_resource.actions",
	"data.permitio_resource.actions.description",
	"data.permitio_resource.actions.id",
	"data.permitio_resource.actions.name",
	"data.permitio_resource.attributes",
	"data.permitio_resource.attributes.description",
	"data.permitio_resource.attributes.type",
	"data.permitio_resource.created_at",
	"data.permitio_resource.description",
	"data.permitio_resource.environment_id",
	"data.permitio_resource.id",
	"data.permitio_resource.key",
	"data.permitio_resource.name",
	"data.permitio_resource.organization_id",
	"data.permitio_resource.project_id",
	"data.permitio_resource.updated_at",
	"data.permitio_resource.urn",
	"data.permitio_role.created_at",
	"data.permitio_role.description",
	"data.permitio_role.environment_id",
	"data.permitio_role.extends",
	"data.permitio_role.id",
	"data.permitio_role.key",
	"data.permitio_role.name",
	"data.permitio_role.organization_id",
	"data.permitio_role.permissions",
	"data.permitio_role.project_id",
	"data.permitio_role.resource",
	"data.permitio_role.resource_id",
	"data.permitio_role.updated_at",
	"permitio_proxy_config.auth_secret.basic",
	"permitio_proxy_config.auth_secret.bearer",
	"permitio_proxy_config.auth_secret.headers",
	"permitio_proxy_config.mapping_rules.action",
	"permitio_proxy_config.mapping_rules.headers",
	"permitio_proxy_config.mapping_rules.http_method",
	"permitio_proxy_config.mapping_rules.priority",
	"permitio_proxy_config.mapping_rules.resource",
	"permitio_proxy_config.mapping_rules.url",
	"permitio_resource.actions.description",
	"permitio_resource.actions.id",
	"permitio_resource.actions.name",
	"permitio_resource.attributes.description",
	"permitio_resource.attributes.type",
	"permitio_resource_instance_role_assignment.created_at",
	"permitio_resource_instance_role_assignment.environment_id",
	"permitio_resource_instance_role_assignment.organization_id",
	"permitio_resource_instance_role_assignment.project_id",
	"permitio_resource_set.resource",
	"permitio_role_assignment.created_at",
	"permitio_role_assignment.environment_id",
	"permitio_role_assignment.organization_id",
	"permitio_role_assignment.project_id",
}

// TestSchemaAttributesHaveDescriptions fails for every resource or data source
// attribute that has an empty description and is not in undescribedAttributes,
// and for every entry of undescribedAttributes that now has a description or no
// longer exists, so that the list can only shrink.
func TestSchemaAttributesHaveDescriptions(t *testing.T) {
	snapshot, err := providerschema.Build(t.Context(), New("test")())
	if err != nil {
		t.Fatalf("building the provider schema snapshot: %v", err)
	}
	if len(snapshot.Resources) != wantResourceCount ||
		len(snapshot.DataSources) != wantDataSourceCount {
		t.Fatalf("DID NOT RUN: the provider serves %d resources and %d data sources, "+
			"want %d and %d", len(snapshot.Resources), len(snapshot.DataSources),
			wantResourceCount, wantDataSourceCount)
	}
	if !slices.IsSorted(undescribedAttributes) {
		t.Error("undescribedAttributes is not sorted")
	}

	undescribed := map[string]bool{}
	checked := 0
	for prefix, schemas := range map[string]map[string]providerschema.Schema{
		"":      snapshot.Resources,
		"data.": snapshot.DataSources,
	} {
		for name, schema := range schemas {
			if len(schema.Attributes) == 0 {
				t.Errorf("DID NOT RUN: %s%s has no attributes", prefix, name)
			}
			for attributePath, attribute := range schema.Attributes {
				checked++
				if strings.TrimSpace(attribute.Description) == "" {
					undescribed[prefix+name+"."+attributePath] = true
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("DID NOT RUN: no attribute was checked")
	}

	for _, address := range slices.Sorted(maps.Keys(undescribed)) {
		if !slices.Contains(undescribedAttributes, address) {
			t.Errorf("%s has no description; give it one", address)
		}
	}
	for _, address := range undescribedAttributes {
		if !undescribed[address] {
			t.Errorf("%s has a description or no longer exists; remove it from "+
				"undescribedAttributes", address)
		}
	}
}
