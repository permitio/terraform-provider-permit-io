// Package config hands the provider's connection to the Permit API to the resources
// that send their own HTTP requests instead of calling the SDK.
package config

import (
	"net/http"
	"sync/atomic"
)

// API is the provider's connection to the Permit API, as its Configure resolved it
// for the SDK client: the HTTP client, with the provider's retries, User-Agent and
// timeout, the base URL and API key, and the IDs of the project and environment
// that the API key's scope names. The IDs are empty for an API key that is not
// scoped to one environment.
type API struct {
	HTTPClient    *http.Client
	URL           string
	Key           string
	ProjectID     string
	EnvironmentID string
}

// current is the connection of the last provider Configure in this process.
// Terraform runs each provider configuration in its own plugin process, except when
// the provider is reattached, as in the offline tests and in debugging. There the
// last Configure wins, as it does for the SDK client the framework's ResourceData
// hands to the resources.
var current atomic.Pointer[API]

// SetAPI stores the provider's connection. The provider's Configure calls it.
func SetAPI(api API) {
	current.Store(&api)
}

// GetAPI returns the connection SetAPI stored, or nil when the provider has not
// been configured.
func GetAPI() *API {
	return current.Load()
}
