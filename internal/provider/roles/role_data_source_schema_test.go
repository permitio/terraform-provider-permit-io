package roles

import (
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/datasourcetest"
)

// TestRoleDataSourceConfigDecodes checks that the permitio_role data source's
// schema decodes into the model its Read uses.
func TestRoleDataSourceConfigDecodes(t *testing.T) {
	datasourcetest.CheckConfigDecodes(t, NewRoleDataSource(), &roleModel{})
}
