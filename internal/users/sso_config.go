package users

import (
	"fmt"
	"os"
	"strings"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/microsoftonline"
	"github.com/markbates/goth/providers/openidConnect"
)

// SSOManager manages SSO providers and configuration
type SSOManager struct {
	config    *SSOConfig
	providers map[string]goth.Provider
}

// NewSSOManager creates a new SSO manager instance
func NewSSOManager() *SSOManager {
	return &SSOManager{
		providers: make(map[string]goth.Provider),
	}
}

// LoadConfigFromEnvironment loads SSO configuration from environment variables
func (m *SSOManager) LoadConfigFromEnvironment() error {
	config := &SSOConfig{
		Providers: make(map[string]SSOProviderConfig),
	}

	// Load Azure AD / Microsoft Entra ID configuration
	if clientID := os.Getenv("AZURE_AD_CLIENT_ID"); clientID != "" {
		config.Providers["azuread"] = SSOProviderConfig{
			Type:         "oidc",
			ClientID:     clientID,
			ClientSecret: os.Getenv("AZURE_AD_CLIENT_SECRET"),
			MetadataURL:  getAzureADMetadataURL(),
			Enabled:      os.Getenv("AZURE_AD_ENABLED") != "false",
			CallbackURL:  getSSOCallbackURL("azuread"),
			GroupMapping: parseGroupMapping(os.Getenv("AZURE_AD_GROUP_MAPPING")),
			Scopes:       []string{"openid", "profile", "email", "User.Read"},
		}
	}

	// Load generic OIDC configuration
	if clientID := os.Getenv("OIDC_CLIENT_ID"); clientID != "" {
		config.Providers["oidc"] = SSOProviderConfig{
			Type:         "oidc",
			ClientID:     clientID,
			ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
			MetadataURL:  os.Getenv("OIDC_METADATA_URL"),
			Enabled:      os.Getenv("OIDC_ENABLED") != "false",
			CallbackURL:  getSSOCallbackURL("oidc"),
			GroupMapping: parseGroupMapping(os.Getenv("OIDC_GROUP_MAPPING")),
			Scopes:       parseScopes(os.Getenv("OIDC_SCOPES")),
		}
	}

	// Load SAML configuration
	if entityID := os.Getenv("SAML_ENTITY_ID"); entityID != "" {
		config.Providers["saml"] = SSOProviderConfig{
			Type:         "saml",
			EntityID:     entityID,
			SSOURL:       os.Getenv("SAML_SSO_URL"),
			Certificate:  os.Getenv("SAML_CERTIFICATE"),
			MetadataURL:  os.Getenv("SAML_METADATA_URL"),
			Enabled:      os.Getenv("SAML_ENABLED") != "false",
			CallbackURL:  getSSOCallbackURL("saml"),
			GroupMapping: parseGroupMapping(os.Getenv("SAML_GROUP_MAPPING")),
		}
	}

	m.config = config
	return m.initializeProviders()
}

// getAzureADMetadataURL constructs the Azure AD metadata URL
func getAzureADMetadataURL() string {
	tenantID := os.Getenv("AZURE_AD_TENANT_ID")
	if tenantID == "" {
		tenantID = "common" // Default to multi-tenant
	}
	return fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0/.well-known/openid_configuration", tenantID)
}

// getSSOCallbackURL constructs the callback URL for an SSO provider
func getSSOCallbackURL(provider string) string {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080" // Default for development
	}
	return fmt.Sprintf("%s/auth/sso/%s/callback", baseURL, provider)
}

// parseGroupMapping parses group mapping from environment variable
// Format: "group1:role1,group2:role2"
func parseGroupMapping(mapping string) map[string]string {
	result := make(map[string]string)
	if mapping == "" {
		return result
	}

	pairs := strings.Split(mapping, ",")
	for _, pair := range pairs {
		parts := strings.Split(strings.TrimSpace(pair), ":")
		if len(parts) == 2 {
			group := strings.TrimSpace(parts[0])
			role := strings.TrimSpace(parts[1])
			if group != "" && role != "" {
				result[group] = role
			}
		}
	}
	return result
}

// parseScopes parses scopes from environment variable
// Format: "scope1,scope2,scope3"
func parseScopes(scopes string) []string {
	if scopes == "" {
		return []string{"openid", "profile", "email"}
	}

	result := make([]string, 0)
	for _, scope := range strings.Split(scopes, ",") {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			result = append(result, scope)
		}
	}
	return result
}

// initializeProviders initializes Goth providers based on SSO configuration
func (m *SSOManager) initializeProviders() error {
	var providers []goth.Provider

	for name, config := range m.config.Providers {
		if !config.Enabled {
			continue
		}

		var provider goth.Provider
		var err error

		switch config.Type {
		case "oidc":
			if name == "azuread" {
				// Use Azure AD specific provider
				provider, err = m.createAzureADProvider(config)
			} else {
				// Use generic OIDC provider
				provider, err = m.createOIDCProvider(name, config)
			}
		case "saml":
			// SAML provider would be implemented here
			// For now, we'll skip SAML as it requires additional dependencies
			fmt.Printf("Warning: SAML provider %s not yet implemented, skipping\n", name)
			continue
		default:
			fmt.Printf("Warning: Unknown SSO provider type %s for %s, skipping\n", config.Type, name)
			continue
		}

		if err != nil {
			return fmt.Errorf("failed to initialize SSO provider %s: %w", name, err)
		}

		providers = append(providers, provider)
		m.providers[name] = provider
		fmt.Printf("SSO provider %s (%s) initialized successfully\n", name, config.Type)
	}

	// Add SSO providers to existing OAuth providers
	if len(providers) > 0 {
		existingProviders := goth.GetProviders()
		allProviders := make([]goth.Provider, 0, len(existingProviders)+len(providers))

		// Add existing providers
		for _, provider := range existingProviders {
			allProviders = append(allProviders, provider)
		}

		// Add new SSO providers
		allProviders = append(allProviders, providers...)

		goth.UseProviders(allProviders...)
	}

	return nil
}

// createAzureADProvider creates an Azure AD provider using Microsoft Online
func (m *SSOManager) createAzureADProvider(config SSOProviderConfig) (goth.Provider, error) {
	if config.ClientID == "" || config.ClientSecret == "" {
		return nil, fmt.Errorf("Azure AD client ID and secret are required")
	}

	tenantID := os.Getenv("AZURE_AD_TENANT_ID")
	if tenantID == "" {
		tenantID = "common"
	}

	provider := microsoftonline.New(
		config.ClientID,
		config.ClientSecret,
		config.CallbackURL,
		config.Scopes...,
	)

	return provider, nil
}

// createOIDCProvider creates a generic OIDC provider
func (m *SSOManager) createOIDCProvider(name string, config SSOProviderConfig) (goth.Provider, error) {
	if config.ClientID == "" || config.ClientSecret == "" {
		return nil, fmt.Errorf("OIDC client ID and secret are required")
	}

	if config.MetadataURL == "" {
		return nil, fmt.Errorf("OIDC metadata URL is required")
	}

	provider, err := openidConnect.New(
		config.ClientID,
		config.ClientSecret,
		config.CallbackURL,
		config.MetadataURL,
		config.Scopes...,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// Set provider name for identification
	provider.SetName(name)

	return provider, nil
}

// GetProvider returns an SSO provider by name
func (m *SSOManager) GetProvider(name string) (goth.Provider, error) {
	provider, exists := m.providers[name]
	if !exists {
		return nil, fmt.Errorf("SSO provider %s not found or not enabled", name)
	}
	return provider, nil
}

// GetEnabledProviders returns a list of enabled SSO provider names
func (m *SSOManager) GetEnabledProviders() []string {
	var providers []string
	for name, config := range m.config.Providers {
		if config.Enabled && m.providers[name] != nil {
			providers = append(providers, name)
		}
	}
	return providers
}

// IsProviderEnabled checks if an SSO provider is enabled
func (m *SSOManager) IsProviderEnabled(name string) bool {
	config, exists := m.config.Providers[name]
	if !exists {
		return false
	}
	return config.Enabled && m.providers[name] != nil
}

// ValidateProviderConfig validates SSO provider configuration
func (m *SSOManager) ValidateProviderConfig(name string) error {
	config, exists := m.config.Providers[name]
	if !exists {
		return fmt.Errorf("SSO provider %s not configured", name)
	}

	if !config.Enabled {
		return fmt.Errorf("SSO provider %s is disabled", name)
	}

	switch config.Type {
	case "oidc":
		if config.ClientID == "" {
			return fmt.Errorf("SSO provider %s missing client ID", name)
		}
		if config.ClientSecret == "" {
			return fmt.Errorf("SSO provider %s missing client secret", name)
		}
		if config.MetadataURL == "" {
			return fmt.Errorf("SSO provider %s missing metadata URL", name)
		}
	case "saml":
		if config.EntityID == "" {
			return fmt.Errorf("SSO provider %s missing entity ID", name)
		}
		if config.SSOURL == "" && config.MetadataURL == "" {
			return fmt.Errorf("SSO provider %s missing SSO URL or metadata URL", name)
		}
	default:
		return fmt.Errorf("SSO provider %s has unsupported type: %s", name, config.Type)
	}

	if config.CallbackURL == "" {
		return fmt.Errorf("SSO provider %s missing callback URL", name)
	}

	return nil
}

// GetProviderConfig returns the configuration for an SSO provider
func (m *SSOManager) GetProviderConfig(name string) (*SSOProviderConfig, error) {
	config, exists := m.config.Providers[name]
	if !exists {
		return nil, fmt.Errorf("SSO provider %s not found", name)
	}
	return &config, nil
}

// MapGroupsToRoles maps SSO groups to Tickit roles based on configuration
func (m *SSOManager) MapGroupsToRoles(provider string, groups []string) []string {
	config, exists := m.config.Providers[provider]
	if !exists || len(config.GroupMapping) == 0 {
		return []string{"Viewer"} // Default role
	}

	roleSet := make(map[string]bool)

	for _, group := range groups {
		if role, exists := config.GroupMapping[group]; exists {
			roleSet[role] = true
		}
	}

	// Convert set to slice
	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}

	// If no roles mapped, return default
	if len(roles) == 0 {
		return []string{"Viewer"}
	}

	return roles
}

// GetHighestRole returns the highest role from a list of roles
// Role hierarchy: Admin > ProjectManager > Developer > Viewer
func (m *SSOManager) GetHighestRole(roles []string) string {
	roleHierarchy := map[string]int{
		"Admin":          4,
		"ProjectManager": 3,
		"Developer":      2,
		"Viewer":         1,
	}

	highestRole := "Viewer"
	highestLevel := 0

	for _, role := range roles {
		if level, exists := roleHierarchy[role]; exists && level > highestLevel {
			highestRole = role
			highestLevel = level
		}
	}

	return highestRole
}

// GetSupportedSSOProviders returns a list of all supported SSO provider names
func GetSupportedSSOProviders() []string {
	return []string{"azuread", "oidc", "saml"}
}

// IsSSOProviderSupported checks if an SSO provider is supported by the system
func IsSSOProviderSupported(provider string) bool {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	for _, supported := range GetSupportedSSOProviders() {
		if supported == normalized {
			return true
		}
	}
	return false
}
