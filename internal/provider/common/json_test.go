package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
)

func TestDecodeJSONObject(t *testing.T) {
	object, err := DecodeJSONObject(`{"account": 9007199254740993, "tier": "gold"}`)
	if err != nil {
		t.Fatalf("DecodeJSONObject() = %v", err)
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"account":9007199254740993,"tier":"gold"}`; string(encoded) != want {
		t.Errorf("DecodeJSONObject() encodes as %s, want %s", encoded, want)
	}

	for _, value := range []string{"null", "[1]", `"gold"`, "not json"} {
		if _, err := DecodeJSONObject(value); err == nil ||
			!strings.Contains(err.Error(), "decoding the JSON "+value) {
			t.Errorf("DecodeJSONObject(%s) = %v, want an error decoding it", value, err)
		}
	}
}

func TestOptionalJSONObjectValue(t *testing.T) {
	null := jsontypes.NewNormalizedNull()
	tests := []struct {
		name   string
		object map[string]any
		prior  jsontypes.Normalized
		want   jsontypes.Normalized
	}{
		{name: "no object, prior null", prior: null, want: null},
		{
			name:  "no object, prior an empty object",
			prior: jsontypes.NewNormalizedValue("{}"), want: jsontypes.NewNormalizedValue("{}"),
		},
		{name: "empty object, prior null", object: map[string]any{}, prior: null, want: null},
		{
			name: "empty object, prior an empty object", object: map[string]any{},
			prior: jsontypes.NewNormalizedValue("{ }"), want: jsontypes.NewNormalizedValue("{ }"),
		},
		{
			name: "same object, numbers written another way",
			// The SDK decodes 9007199254740993 as the float64 9007199254740992.
			object: map[string]any{"account": float64(9007199254740993), "seats": float64(25)},
			prior:  jsontypes.NewNormalizedValue(`{"seats": 25.0, "account": 9007199254740993}`),
			want:   jsontypes.NewNormalizedValue(`{"seats": 25.0, "account": 9007199254740993}`),
		},
		{
			name:   "other object",
			object: map[string]any{"tier": "gold"},
			prior:  jsontypes.NewNormalizedValue(`{"tier": "silver"}`),
			want:   jsontypes.NewNormalizedValue(`{"tier":"gold"}`),
		},
		{
			name:   "object, prior null",
			object: map[string]any{"tier": "gold"},
			prior:  null,
			want:   jsontypes.NewNormalizedValue(`{"tier":"gold"}`),
		},
		{
			name:   "empty object, prior an object",
			object: map[string]any{},
			prior:  jsontypes.NewNormalizedValue(`{"tier": "gold"}`),
			want:   null,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OptionalJSONObjectValue(tt.object, tt.prior)
			if err != nil {
				t.Fatalf("OptionalJSONObjectValue() = %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("OptionalJSONObjectValue() = %s, want %s", got, tt.want)
			}
		})
	}
}
