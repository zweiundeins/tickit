package users

import (
	"fmt"
	"os"
	"strings"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"
	"github.com/markbates/goth/providers/microsoftonline"
)

// OAuthConfig represents the OAuth configuration
type OAuthConfig struct {
	Providers map[string]OAuthProviderConfig `json:"providers"`
}

// OAuthProviderConfig represents configuration for a single OAuth provider
type OAuthProviderConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	Enabled      bool     `json:"enabled"`
	CallbackURL  string   `json:"callback_url"`
}

// OAuthManager manages OAuth providers and configuration
type OAuthManager struct {
	config    *OAuthConfig
	providers map[string]goth.Provider
}

// NewOAuthManager creates a new OAuth manager instance
func NewOAuthManager() *OAuthManager {
	return &OAuthManager{
		providers: make(map[string]goth.Provider),
	}
}

// LoadConfigFromEnvironment loads OAuth configuration from environment variables
func (m *OAuthManager) LoadConfigFromEnvironment() error {
	config := &OAuthConfig{
		Providers: make(map[string]OAuthProviderConfig),
	}

	// Load GitHub configuration
	if clientID := os.Getenv("GITHUB_CLIENT_ID"); clientID != "" {
		config.Providers["github"] = OAuthProviderConfig{
			ClientID:     clientID,
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			Scopes:       []string{"user:email"},
			Enabled:      os.Getenv("GITHUB_ENABLED") != "false", // Default to enabled if not explicitly disabled
			CallbackURL:  getCallbackURL("github"),
		}
	}

	// Load Google configuration
	if clientID := os.Getenv("GOOGLE_CLIENT_ID"); clientID != "" {
		config.Providers["google"] = OAuthProviderConfig{
			ClientID:     clientID,
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			Scopes:       []string{"email", "profile"},
			Enabled:      os.Getenv("GOOGLE_ENABLED") != "false",
			CallbackURL:  getCallbackURL("google"),
		}
	}

	// Load Microsoft configuration
	if clientID := os.Getenv("MICROSOFT_CLIENT_ID"); clientID != "" {
		config.Providers["microsoft"] = OAuthProviderConfig{
			ClientID:     clientID,
			ClientSecret: os.Getenv("MICROSOFT_CLIENT_SECRET"),
			Scopes:       []string{"openid", "profile", "email"},
			Enabled:      os.Getenv("MICROSOFT_ENABLED") != "false",
			CallbackURL:  getCallbackURL("microsoft"),
		}
	}

	m.config = config
	return m.initializeProviders()
}

// getCallbackURL constructs the callback URL for a provider
func getCallbackURL(provider string) string {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080" // Default for development
	}
	return fmt.Sprintf("%s/auth/oauth/%s/callback", baseURL, provider)
}

// initializeProviders initializes Goth providers based on configuration
func (m *OAuthManager) initializeProviders() error {
	var providers []goth.Provider

	for name, config := range m.config.Providers {
		if !config.Enabled {
			continue
		}

		if config.ClientID == "" || config.ClientSecret == "" {
			fmt.Printf("Warning: OAuth provider %s is missing client ID or secret, skipping\n", name)
			continue
		}

		var provider goth.Provider
		var err error

		switch name {
		case "github":
			provider = github.New(
				config.ClientID,
				config.ClientSecret,
				config.CallbackURL,
				config.Scopes...,
			)
		case "google":
			provider = google.New(
				config.ClientID,
				config.ClientSecret,
				config.CallbackURL,
				config.Scopes...,
			)
		case "microsoft":
			provider = microsoftonline.New(
				config.ClientID,
				config.ClientSecret,
				config.CallbackURL,
				config.Scopes...,
			)
		default:
			fmt.Printf("Warning: Unknown OAuth provider %s, skipping\n", name)
			continue
		}

		if err != nil {
			return fmt.Errorf("failed to initialize %s provider: %w", name, err)
		}

		providers = append(providers, provider)
		m.providers[name] = provider
		fmt.Printf("OAuth provider %s initialized successfully\n", name)
	}

	// Set providers in Goth
	goth.UseProviders(providers...)
	return nil
}

// GetProvider returns a provider by name
func (m *OAuthManager) GetProvider(name string) (goth.Provider, error) {
	provider, exists := m.providers[name]
	if !exists {
		return nil, fmt.Errorf("OAuth provider %s not found or not enabled", name)
	}
	return provider, nil
}

// GetEnabledProviders returns a list of enabled provider names
func (m *OAuthManager) GetEnabledProviders() []string {
	var providers []string
	for name, config := range m.config.Providers {
		if config.Enabled && m.providers[name] != nil {
			providers = append(providers, name)
		}
	}
	return providers
}

// IsProviderEnabled checks if a provider is enabled
func (m *OAuthManager) IsProviderEnabled(name string) bool {
	config, exists := m.config.Providers[name]
	if !exists {
		return false
	}
	return config.Enabled && m.providers[name] != nil
}

// ValidateProviderConfig validates OAuth provider configuration
func (m *OAuthManager) ValidateProviderConfig(name string) error {
	config, exists := m.config.Providers[name]
	if !exists {
		return fmt.Errorf("provider %s not configured", name)
	}

	if !config.Enabled {
		return fmt.Errorf("provider %s is disabled", name)
	}

	if config.ClientID == "" {
		return fmt.Errorf("provider %s missing client ID", name)
	}

	if config.ClientSecret == "" {
		return fmt.Errorf("provider %s missing client secret", name)
	}

	if config.CallbackURL == "" {
		return fmt.Errorf("provider %s missing callback URL", name)
	}

	return nil
}

// GetProviderConfig returns the configuration for a provider
func (m *OAuthManager) GetProviderConfig(name string) (*OAuthProviderConfig, error) {
	config, exists := m.config.Providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}
	return &config, nil
}

// NormalizeProviderName normalizes provider names to lowercase
func NormalizeProviderName(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// GetSupportedProviders returns a list of all supported provider names
func GetSupportedProviders() []string {
	return []string{"github", "google", "microsoft"}
}

// IsProviderSupported checks if a provider is supported by the system
func IsProviderSupported(provider string) bool {
	normalized := NormalizeProviderName(provider)
	for _, supported := range GetSupportedProviders() {
		if supported == normalized {
			return true
		}
	}
	return false
}
