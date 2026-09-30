package user_attributes

import (
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/datasourcetest"
)

// TestUserAttributeDataSourceConfigDecodes checks that the permitio_user_attribute
// data source's schema decodes into the model its Read uses.
func TestUserAttributeDataSourceConfigDecodes(t *testing.T) {
	datasourcetest.CheckConfigDecodes(t, NewUserAttributeDataSource(), &userAttributeModel{})
}
