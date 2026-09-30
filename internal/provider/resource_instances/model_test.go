package resource_instances

import (
	"math"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
)

// TestResourceInstanceModelReportsAttributesItCannotEncode checks that attributes
// that do not encode as JSON give an error instead of the attributes "{}".
func TestResourceInstanceModelReportsAttributesItCannotEncode(t *testing.T) {
	_, err := tfModelFromResourceInstanceRead(models.ResourceInstanceRead{
		Key:        "handbook",
		Attributes: map[string]interface{}{"score": math.Inf(1)},
	})

	if err == nil || !strings.Contains(err.Error(), "encoding the attributes") {
		t.Errorf("tfModelFromResourceInstanceRead = %v, want an error encoding the attributes",
			err)
	}
}
