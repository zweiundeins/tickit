package users

import "time"

// SSOConfig represents the SSO configuration for enterprise authentication
type SSOConfig struct {
	Providers map[string]SSOProviderConfig `json:"providers"`
}

// SSOProviderConfig represents configuration for a single SSO provider
type SSOProviderConfig struct {
	Type         string            `json:"type"`          // "saml" or "oidc"
	ClientID     string            `json:"client_id"`     // For OIDC
	ClientSecret string            `json:"client_secret"` // For OIDC
	MetadataURL  string            `json:"metadata_url"`  // For SAML/OIDC metadata
	EntityID     string            `json:"entity_id"`     // For SAML
	SSOURL       string            `json:"sso_url"`       // For SAML
	Certificate  string            `json:"certificate"`   // For SAML
	Enabled      bool              `json:"enabled"`
	CallbackURL  string            `json:"callback_url"`
	GroupMapping map[string]string `json:"group_mapping"` // Maps SSO groups to Tickit roles
	Scopes       []string          `json:"scopes"`        // For OIDC
}

// SSOSession represents a temporary SSO session during authentication
type SSOSession struct {
	ID        string                 `json:"id"`
	State     string                 `json:"state"`
	Provider  string                 `json:"provider"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt time.Time              `json:"expires_at"`
	Metadata  map[string]interface{} `json:"metadata"` // Additional session data
}

// SSOUserAttributes represents user attributes from SSO provider
type SSOUserAttributes struct {
	Email      string            `json:"email"`
	FirstName  string            `json:"first_name"`
	LastName   string            `json:"last_name"`
	Username   string            `json:"username"`
	Groups     []string          `json:"groups"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes"` // Additional attributes
}

// SSOLogoutRequest represents a single logout request
type SSOLogoutRequest struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Provider   string    `json:"provider"`
	LogoutURL  string    `json:"logout_url"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Completed  bool      `json:"completed"`
	SessionIDs []string  `json:"session_ids"` // Sessions to invalidate
}

// SSOAuditLog represents SSO authentication events for auditing
type SSOAuditLog struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Provider  string                 `json:"provider"`
	Event     string                 `json:"event"` // "login", "logout", "group_sync", "error"
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent"`
	Details   map[string]interface{} `json:"details"`
	CreatedAt time.Time              `json:"created_at"`
}
