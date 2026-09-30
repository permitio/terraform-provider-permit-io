package config

import (
	"net/http"
	"testing"
)

func TestSetAndGetAPI(t *testing.T) {
	current.Store(nil)
	if got := GetAPI(); got != nil {
		t.Fatalf("GetAPI() before SetAPI = %+v, want nil", got)
	}

	client := &http.Client{}
	want := API{
		HTTPClient: client, URL: "https://api.example.com", Key: "key",
		ProjectID: "project", EnvironmentID: "environment",
	}
	SetAPI(want)

	if got := GetAPI(); got == nil || *got != want {
		t.Errorf("GetAPI() = %+v, want the connection SetAPI stored, %+v", got, want)
	}
}
