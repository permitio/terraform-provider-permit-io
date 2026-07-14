package user_attributes

import (
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
)

func TestTfModelFromSDKNilDescription(t *testing.T) {
	// Attributes created outside Terraform (dashboard/API) may have no description.
	sdkModel := models.ResourceAttributeRead{
		Type:        models.STRING,
		Description: nil,
		Key:         "no_description",
		Id:          "6b9c1f0e0e0e4a0e8f0e0e0e0e0e0e0e",
		ResourceId:  "5a8b1f0e0e0e4a0e8f0e0e0e0e0e0e0e",
		ResourceKey: UserKey,
	}

	model := tfModelFromSDK(sdkModel)

	if !model.Description.IsNull() {
		t.Errorf("expected null description, got %q", model.Description.ValueString())
	}
	if model.Key.ValueString() != "no_description" {
		t.Errorf("expected key %q, got %q", "no_description", model.Key.ValueString())
	}
}
