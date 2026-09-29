package conditionsetrules

import "testing"

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
