package conditionsets

import (
	"testing"

	"github.com/permitio/terraform-provider-permit-io/internal/acctest/datasourcetest"
)

// TestConditionSetDataSourceConfigDecodes checks that the permitio_condition_set
// data source's schema decodes into the model its Read uses.
func TestConditionSetDataSourceConfigDecodes(t *testing.T) {
	datasourcetest.CheckConfigDecodes(t, NewConditionSetDataSource(),
		&conditionSetDataSourceModel{})
}
