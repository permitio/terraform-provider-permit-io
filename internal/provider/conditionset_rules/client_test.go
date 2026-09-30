package conditionsetrules

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/permit"
)

// TestCreateFailsWhenTheAnswerHasNoRule answers the create of a rule with an
// empty list, and checks that Create reports it instead of indexing the list.
func TestCreateFailsWhenTheAnswerHasNoRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v2/api-key/scope":
			_, _ = io.WriteString(w,
				`{"organization_id":"org","project_id":"proj","environment_id":"env"}`)
		case "POST /v2/facts/proj/env/set_rules":
			_, _ = io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sdkConfig := config.NewConfigBuilder("test-key").WithApiUrl(server.URL).Build()
	client := ConditionSetRuleClient{client: permit.NewPermit(sdkConfig)}
	plan := ConditionSetRuleModel{
		UserSet:     types.StringValue("reviewers"),
		Permission:  types.StringValue("document:read"),
		ResourceSet: types.StringValue("drafts"),
	}

	err := client.Create(t.Context(), &plan)

	if err == nil || !strings.Contains(err.Error(), "no condition set rule") {
		t.Errorf("Create = %v, want an error that the answer has no condition set rule", err)
	}
}

func TestPermissionFilterValue(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		want       string
	}{
		{"resource action key", "document:read", "read"},
		{"workspace action key", "ws:access", "access"},
		{"bare action key", "read", "read"},
		// Resource-action ids have no colon and must pass through untouched.
		{"resource action id", "a1b2c3d4e5f6", "a1b2c3d4e5f6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := permissionFilterValue(tt.permission); got != tt.want {
				t.Errorf("permissionFilterValue(%q) = %q, want %q", tt.permission, got, tt.want)
			}
		})
	}
}
