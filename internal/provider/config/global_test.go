package config

import "testing"

func TestSetAndGetGlobalConfig(t *testing.T) {
	testApiUrl := "https://test.api.permit.io"
	testApiKey := "test-api-key-123"

	SetGlobalConfig(testApiUrl, testApiKey)

	if got := GetGlobalApiUrl(); got != testApiUrl {
		t.Errorf("GetGlobalApiUrl() = %v, want %v", got, testApiUrl)
	}

	if got := GetGlobalApiKey(); got != testApiKey {
		t.Errorf("GetGlobalApiKey() = %v, want %v", got, testApiKey)
	}
}

func TestGetGlobalConfigEmpty(t *testing.T) {
	// Reset globals
	globalApiUrl = ""
	globalApiKey = ""

	if got := GetGlobalApiUrl(); got != "" {
		t.Errorf("GetGlobalApiUrl() should be empty, got %v", got)
	}

	if got := GetGlobalApiKey(); got != "" {
		t.Errorf("GetGlobalApiKey() should be empty, got %v", got)
	}
}
