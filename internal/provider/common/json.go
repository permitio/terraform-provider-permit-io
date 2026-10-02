package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
)

// errNotJSONObject is the error for a JSON attribute that holds valid JSON other
// than an object, such as null or a list.
var errNotJSONObject = errors.New("want a JSON object, such as jsonencode({ ... }) returns")

// DecodeJSONObject decodes value, the JSON object a JSON attribute holds, for a
// request to the Permit API. It keeps each number as the json.Number the
// configuration wrote, which the SDK encodes as written, so an integer beyond 2^53
// reaches the API exact instead of rounded to a float64.
func DecodeJSONObject(value string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("decoding the JSON %s: %w", value, err)
	}
	if object == nil {
		return nil, fmt.Errorf("decoding the JSON %s: %w", value, errNotJSONObject)
	}
	return object, nil
}

// JSONObjectValue returns object, a JSON object the SDK decoded from a Permit API
// answer, as the value of a JSON attribute whose plan or prior state holds prior.
// The SDK decodes every number as a float64, which rounds an integer beyond 2^53.
// So when prior decodes the same way to the same object, JSONObjectValue returns
// prior, with the exact numbers, key order and whitespace the configuration wrote.
func JSONObjectValue(object map[string]any, prior jsontypes.Normalized) (
	jsontypes.Normalized, error,
) {
	if holdsObject(prior, object) {
		return prior, nil
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return jsontypes.Normalized{}, fmt.Errorf("encoding the JSON object: %w", err)
	}
	return jsontypes.NewNormalizedValue(string(encoded)), nil
}

// OptionalJSONObjectValue is JSONObjectValue for an optional attribute. It is null
// when the API holds no object or an empty one, unless prior is an empty object,
// so that an attribute left out of the configuration stays null.
func OptionalJSONObjectValue(object map[string]any, prior jsontypes.Normalized) (
	jsontypes.Normalized, error,
) {
	if len(object) == 0 && !holdsObject(prior, object) {
		return jsontypes.NewNormalizedNull(), nil
	}
	return JSONObjectValue(object, prior)
}

// holdsObject reports whether prior is a JSON object that decodes to object the way
// the SDK decodes an API answer, with numbers as float64. An API answer without
// the object and one with an empty object are the same.
func holdsObject(prior jsontypes.Normalized, object map[string]any) bool {
	if prior.IsNull() || prior.IsUnknown() {
		return false
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(prior.ValueString()), &decoded); err != nil ||
		decoded == nil {
		return false
	}
	if len(decoded) == 0 && len(object) == 0 {
		return true
	}
	return reflect.DeepEqual(decoded, object)
}
