package users

import (
	"math"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
)

// TestUserModelReportsAttributesItCannotEncode checks that attributes that do not
// encode as JSON give an error instead of the attributes "{}".
func TestUserModelReportsAttributesItCannotEncode(t *testing.T) {
	_, err := tfModelFromUserRead(models.UserRead{
		Key:        "alice",
		Attributes: map[string]interface{}{"score": math.Inf(1)},
	})

	if err == nil || !strings.Contains(err.Error(), "encoding the attributes") {
		t.Errorf("tfModelFromUserRead = %v, want an error encoding the attributes", err)
	}
}
