package users

import (
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/datasourcetest"
)

// TestUserDataSourceConfigDecodes checks that the permitio_user data source's
// schema decodes into the model its Read uses.
func TestUserDataSourceConfigDecodes(t *testing.T) {
	datasourcetest.CheckConfigDecodes(t, NewUserDataSource(), &userModel{})
}
