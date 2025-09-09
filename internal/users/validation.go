package users

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// Email validation regex pattern
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail validates an email address format
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}

	email = strings.TrimSpace(email)
	if len(email) > 254 {
		return fmt.Errorf("email is too long (maximum 254 characters)")
	}

	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// ValidatePassword validates a password according to security requirements
func ValidatePassword(password string) error {
	if password == "" {
		return fmt.Errorf("password is required")
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	if len(password) > 128 {
		return fmt.Errorf("password is too long (maximum 128 characters)")
	}

	// Check for at least one lowercase letter
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	// Check for at least one uppercase letter
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	// Check for at least one digit
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)

	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}

	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}

	return nil
}

// ValidateUsername validates a username
func ValidateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}

	username = strings.TrimSpace(username)
	if len(username) < 2 {
		return fmt.Errorf("username must be at least 2 characters long")
	}

	if len(username) > 50 {
		return fmt.Errorf("username is too long (maximum 50 characters)")
	}

	// Username can contain letters, numbers, underscores, and hyphens
	validUsername := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(username)
	if !validUsername {
		return fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
	}

	return nil
}

// ValidateName validates first name or last name
func ValidateName(name, fieldName string) error {
	if name == "" {
		return nil // Names are optional
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil // Empty after trimming is also allowed
	}

	if len(name) > 100 {
		return fmt.Errorf("%s is too long (maximum 100 characters)", fieldName)
	}

	// Names can contain letters, spaces, hyphens, and apostrophes
	validName := regexp.MustCompile(`^[a-zA-Z\s'-]+$`).MatchString(name)
	if !validName {
		return fmt.Errorf("%s can only contain letters, spaces, hyphens, and apostrophes", fieldName)
	}

	return nil
}

// ValidateAvatarURL validates an avatar URL
func ValidateAvatarURL(avatarURL string) error {
	if avatarURL == "" {
		return nil // Empty avatar URL is allowed (removes avatar)
	}

	avatarURL = strings.TrimSpace(avatarURL)
	if avatarURL == "" {
		return nil // Empty after trimming is also allowed
	}

	if len(avatarURL) > 500 {
		return fmt.Errorf("avatar URL is too long (maximum 500 characters)")
	}

	// Check if it's a valid URL format
	if !strings.HasPrefix(avatarURL, "http://") && !strings.HasPrefix(avatarURL, "https://") && !strings.HasPrefix(avatarURL, "/uploads/") {
		return fmt.Errorf("avatar URL must be a valid HTTP/HTTPS URL or local upload path")
	}

	// Additional validation for local upload paths
	if strings.HasPrefix(avatarURL, "/uploads/") {
		// Ensure it's a safe path (no directory traversal)
		if strings.Contains(avatarURL, "..") {
			return fmt.Errorf("avatar URL contains invalid path characters")
		}

		// Check for valid image extensions
		validExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
		hasValidExtension := false
		lowerURL := strings.ToLower(avatarURL)
		for _, ext := range validExtensions {
			if strings.HasSuffix(lowerURL, ext) {
				hasValidExtension = true
				break
			}
		}

		if !hasValidExtension {
			return fmt.Errorf("avatar URL must point to a valid image file (jpg, jpeg, png, gif, webp)")
		}
	}

	return nil
}

// Enhanced input validation and sanitization functions

// SanitizeString sanitizes a string input to prevent XSS and other attacks
func SanitizeString(input string) string {
	if input == "" {
		return ""
	}

	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Remove control characters except newlines and tabs
	var result strings.Builder
	for _, r := range input {
		if r >= 32 || r == '\n' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// SanitizeHTML sanitizes HTML content to prevent XSS
func SanitizeHTML(input string) string {
	// First sanitize the string
	input = SanitizeString(input)

	// HTML escape to prevent XSS
	return html.EscapeString(input)
}

// SanitizeURL sanitizes a URL to prevent malicious redirects
func SanitizeURL(input string) (string, error) {
	if input == "" {
		return "", nil
	}

	input = SanitizeString(input)

	// Parse URL to validate structure
	parsedURL, err := url.Parse(input)
	if err != nil {
		return "", fmt.Errorf("invalid URL format: %v", err)
	}

	// Only allow http, https, and relative URLs
	if parsedURL.Scheme != "" && parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("invalid URL scheme: %s", parsedURL.Scheme)
	}

	// Prevent javascript: and data: URLs
	if strings.HasPrefix(strings.ToLower(input), "javascript:") ||
		strings.HasPrefix(strings.ToLower(input), "data:") ||
		strings.HasPrefix(strings.ToLower(input), "vbscript:") {
		return "", fmt.Errorf("potentially malicious URL scheme")
	}

	return parsedURL.String(), nil
}

// ValidateStringLength validates string length within bounds
func ValidateStringLength(input, fieldName string, minLength, maxLength int) error {
	length := len(input)

	if length < minLength {
		return fmt.Errorf("%s must be at least %d characters long", fieldName, minLength)
	}

	if length > maxLength {
		return fmt.Errorf("%s must be no more than %d characters long", fieldName, maxLength)
	}

	return nil
}

// ValidateNoSQLInjection checks for potential SQL injection patterns
func ValidateNoSQLInjection(input string) error {
	// Common SQL injection patterns
	sqlPatterns := []string{
		"'", "\"", ";", "--", "/*", "*/", "xp_", "sp_",
		"union", "select", "insert", "update", "delete", "drop",
		"create", "alter", "exec", "execute", "script",
	}

	lowerInput := strings.ToLower(input)
	for _, pattern := range sqlPatterns {
		if strings.Contains(lowerInput, pattern) {
			return fmt.Errorf("input contains potentially malicious content")
		}
	}

	return nil
}

// ValidateNoScriptInjection checks for potential script injection
func ValidateNoScriptInjection(input string) error {
	// Common script injection patterns
	scriptPatterns := []string{
		"<script", "</script>", "javascript:", "vbscript:", "onload=",
		"onerror=", "onclick=", "onmouseover=", "onfocus=", "onblur=",
		"eval(", "setTimeout(", "setInterval(", "Function(",
	}

	lowerInput := strings.ToLower(input)
	for _, pattern := range scriptPatterns {
		if strings.Contains(lowerInput, pattern) {
			return fmt.Errorf("input contains potentially malicious script content")
		}
	}

	return nil
}

// ValidateAlphanumeric validates that input contains only alphanumeric characters
func ValidateAlphanumeric(input, fieldName string) error {
	for _, r := range input {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("%s can only contain letters and numbers", fieldName)
		}
	}
	return nil
}

// ValidateAlphanumericWithSpaces validates alphanumeric with spaces
func ValidateAlphanumericWithSpaces(input, fieldName string) error {
	for _, r := range input {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' {
			return fmt.Errorf("%s can only contain letters, numbers, and spaces", fieldName)
		}
	}
	return nil
}

// ValidateNoRepeatedChars validates that input doesn't have too many repeated characters
func ValidateNoRepeatedChars(input string, maxRepeated int) error {
	if len(input) == 0 {
		return nil
	}

	count := 1
	lastChar := rune(input[0])

	for _, r := range input[1:] {
		if r == lastChar {
			count++
			if count > maxRepeated {
				return fmt.Errorf("input contains too many repeated characters")
			}
		} else {
			count = 1
			lastChar = r
		}
	}

	return nil
}

// ValidateIPAddress validates an IP address format
func ValidateIPAddress(ip string) error {
	if ip == "" {
		return fmt.Errorf("IP address is required")
	}

	// Basic IP validation regex
	ipRegex := regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)

	if !ipRegex.MatchString(ip) {
		return fmt.Errorf("invalid IP address format")
	}

	return nil
}

// ValidateUserAgent validates a user agent string
func ValidateUserAgent(userAgent string) error {
	if userAgent == "" {
		return nil // User agent is optional
	}

	// Check length
	if len(userAgent) > 500 {
		return fmt.Errorf("user agent string is too long")
	}

	// Check for suspicious patterns
	suspiciousPatterns := []string{
		"<script", "javascript:", "vbscript:", "data:",
		"eval(", "setTimeout(", "setInterval(",
	}

	lowerUA := strings.ToLower(userAgent)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerUA, pattern) {
			return fmt.Errorf("user agent contains suspicious content")
		}
	}

	return nil
}

// ValidateToken validates a security token format
func ValidateToken(token, tokenType string) error {
	if token == "" {
		return fmt.Errorf("%s token is required", tokenType)
	}

	// Tokens should be alphanumeric and of reasonable length
	if len(token) < 16 || len(token) > 256 {
		return fmt.Errorf("%s token has invalid length", tokenType)
	}

	// Check for valid token characters (alphanumeric + some special chars)
	tokenRegex := regexp.MustCompile(`^[a-zA-Z0-9\-_\.]+$`)
	if !tokenRegex.MatchString(token) {
		return fmt.Errorf("%s token contains invalid characters", tokenType)
	}

	return nil
}

// ValidateJSONInput validates JSON input for size and structure
func ValidateJSONInput(input []byte, maxSize int) error {
	if len(input) == 0 {
		return fmt.Errorf("JSON input is required")
	}

	if len(input) > maxSize {
		return fmt.Errorf("JSON input too large: %d bytes (max: %d)", len(input), maxSize)
	}

	// Check for potentially malicious JSON patterns
	inputStr := string(input)
	if strings.Contains(inputStr, "__proto__") ||
		strings.Contains(inputStr, "constructor") ||
		strings.Contains(inputStr, "prototype") {
		return fmt.Errorf("JSON input contains potentially malicious properties")
	}

	return nil
}

// ComprehensiveInputValidation performs comprehensive validation on user input
func ComprehensiveInputValidation(input, fieldName string, options ValidationOptions) error {
	// Sanitize first
	input = SanitizeString(input)

	// Check required
	if options.Required && input == "" {
		return fmt.Errorf("%s is required", fieldName)
	}

	if input == "" {
		return nil // Optional field
	}

	// Check length
	if options.MinLength > 0 || options.MaxLength > 0 {
		if err := ValidateStringLength(input, fieldName, options.MinLength, options.MaxLength); err != nil {
			return err
		}
	}

	// Check for SQL injection
	if options.CheckSQLInjection {
		if err := ValidateNoSQLInjection(input); err != nil {
			return err
		}
	}

	// Check for script injection
	if options.CheckScriptInjection {
		if err := ValidateNoScriptInjection(input); err != nil {
			return err
		}
	}

	// Check alphanumeric
	if options.AlphanumericOnly {
		if err := ValidateAlphanumeric(input, fieldName); err != nil {
			return err
		}
	}

	// Check repeated characters
	if options.MaxRepeatedChars > 0 {
		if err := ValidateNoRepeatedChars(input, options.MaxRepeatedChars); err != nil {
			return err
		}
	}

	// Custom regex validation
	if options.CustomRegex != nil {
		if !options.CustomRegex.MatchString(input) {
			return fmt.Errorf("%s format is invalid", fieldName)
		}
	}

	return nil
}

// ValidationOptions holds options for comprehensive input validation
type ValidationOptions struct {
	Required             bool
	MinLength            int
	MaxLength            int
	CheckSQLInjection    bool
	CheckScriptInjection bool
	AlphanumericOnly     bool
	MaxRepeatedChars     int
	CustomRegex          *regexp.Regexp
}
