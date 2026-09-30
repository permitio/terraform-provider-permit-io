package tenants

import (
	"math"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
)

// TestTenantModelReportsAttributesItCannotEncode checks that attributes that do
// not encode as JSON give an error instead of the attributes "{}".
func TestTenantModelReportsAttributesItCannotEncode(t *testing.T) {
	_, err := tfModelFromTenantRead(models.TenantRead{
		Key:        "acme",
		Attributes: map[string]interface{}{"score": math.Inf(1)},
	})

	if err == nil || !strings.Contains(err.Error(), "encoding the attributes") {
		t.Errorf("tfModelFromTenantRead = %v, want an error encoding the attributes", err)
	}
}
