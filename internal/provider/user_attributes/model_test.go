package user_attributes

import (
	"testing"

	"github.com/permitio/permit-golang/pkg/models"
)

func TestTfModelFromSDKDescription(t *testing.T) {
	// Base model without a description, as returned by the API for
	// attributes created outside Terraform (dashboard/raw API).
	base := models.ResourceAttributeRead{
		Type:       models.STRING,
		Key:        "attr",
		Id:         "6b9c1f0e0e0e4a0e8f0e0e0e0e0e0e0e",
		ResourceId: "5a8b1f0e0e0e4a0e8f0e0e0e0e0e0e0e",
	}

	t.Run("nil description maps to null", func(t *testing.T) {
		model := tfModelFromSDK(base)

		if !model.Description.IsNull() {
			t.Errorf("expected null description, got %q", model.Description.ValueString())
		}
		if model.Key.ValueString() != "attr" {
			t.Errorf("expected key %q, got %q", "attr", model.Key.ValueString())
		}
	})

	t.Run("non-nil description is preserved", func(t *testing.T) {
		description := "engineering"
		withDescription := base
		withDescription.Description = &description

		model := tfModelFromSDK(withDescription)

		if model.Description.IsNull() {
			t.Fatal("expected non-null description")
		}
		if model.Description.ValueString() != description {
			t.Errorf("expected description %q, got %q", description, model.Description.ValueString())
		}
	})
}
