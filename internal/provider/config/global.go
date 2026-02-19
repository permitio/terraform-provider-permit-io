package config

// Global config storage for resources that need direct HTTP access
var (
	globalApiUrl string
	globalApiKey string
)

// SetGlobalConfig stores the API URL and key globally
func SetGlobalConfig(apiUrl, apiKey string) {
	globalApiUrl = apiUrl
	globalApiKey = apiKey
}

// GetGlobalApiUrl returns the configured API URL
func GetGlobalApiUrl() string {
	return globalApiUrl
}

// GetGlobalApiKey returns the configured API key
func GetGlobalApiKey() string {
	return globalApiKey
}
