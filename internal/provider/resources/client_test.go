package resources

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/acctest/mockpermit"
)

// TestResourceReadNamesAnUnnamedActionByItsKey reads a resource whose action the
// API returns without a name, which the API allows, and checks that Read names
// the action by its key, as Update does, instead of crashing the provider.
func TestResourceReadNamesAnUnnamedActionByItsKey(t *testing.T) {
	m := mockpermit.New(t, mockpermit.Resources)
	sdk := permit.NewPermit(config.NewConfigBuilder(mockpermit.APIKey).WithApiUrl(m.URL).Build())
	_, err := sdk.Api.Resources.Create(t.Context(), models.ResourceCreate{
		Key:  "document",
		Name: "Document",
		Actions: map[string]models.ActionBlockEditable{
			"read":  {},
			"write": {Name: new("Write")},
		},
	})
	if err != nil {
		t.Fatalf("creating the resource: %v", err)
	}
	client := ResourceClient{client: sdk}

	state, err := client.ResourceRead(t.Context(),
		ResourceModel{Key: types.StringValue("document")})

	if err != nil {
		t.Fatalf("ResourceRead: %v", err)
	}
	for key, want := range map[string]string{"read": "read", "write": "Write"} {
		if got := state.Actions[key].Name; got != types.StringValue(want) {
			t.Errorf("action %s: name = %v, want %q", key, got, want)
		}
	}
}

// TestResourceCreateFailsWhenTheAnswerHasNoActions answers the create of a resource
// with a resource that has no actions, and checks that Create reports it instead
// of leaving the planned actions, whose IDs are unknown, in the state.
func TestResourceCreateFailsWhenTheAnswerHasNoActions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v2/api-key/scope":
			_, _ = io.WriteString(w,
				`{"organization_id":"org","project_id":"proj","environment_id":"env"}`)
		case "POST /v2/schema/proj/env/resources":
			_, _ = io.WriteString(w, `{"key":"document","id":"res-1","name":"Document"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sdkConfig := config.NewConfigBuilder("test-key").WithApiUrl(server.URL).Build()
	client := ResourceClient{client: permit.NewPermit(sdkConfig)}
	plan := ResourceModel{
		Key:  types.StringValue("document"),
		Name: types.StringValue("Document"),
		Actions: map[string]actionsModel{
			"read": {Id: types.StringUnknown(), Name: types.StringValue("Read")},
		},
	}

	err := client.ResourceCreate(t.Context(), &plan)

	if err == nil || !strings.Contains(err.Error(), `resource "document" with no actions`) {
		t.Errorf("ResourceCreate = %v, want an error that the answer has no actions", err)
	}
}
