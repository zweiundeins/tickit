package realtime

import (
	"sync"

	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/users"
)

// RendererRegistry manages the mapping of event patterns to their corresponding renderers
type RendererRegistry struct {
	mu       sync.RWMutex
	patterns map[string]ComponentRenderer // pattern -> renderer
}

// Global renderer registry instance
var globalRendererRegistry = &RendererRegistry{
	patterns: make(map[string]ComponentRenderer),
}

// RegisterRenderer registers a renderer for a specific event pattern
func RegisterRenderer(pattern string, renderer ComponentRenderer) {
	globalRendererRegistry.mu.Lock()
	defer globalRendererRegistry.mu.Unlock()
	globalRendererRegistry.patterns[pattern] = renderer
}

// GetRenderer finds the best matching renderer for an event pattern
// Returns the most specific matching renderer, or nil if no match found
func GetRenderer(eventPattern string) ComponentRenderer {
	globalRendererRegistry.mu.RLock()
	defer globalRendererRegistry.mu.RUnlock()

	// First try exact match
	if renderer, exists := globalRendererRegistry.patterns[eventPattern]; exists {
		return renderer
	}

	// Then try pattern matching (find most specific match)
	var bestMatch ComponentRenderer
	var bestMatchLen int

	for pattern, renderer := range globalRendererRegistry.patterns {
		if matchesPattern(eventPattern, pattern) {
			// Prefer more specific patterns (longer patterns are more specific)
			if len(pattern) > bestMatchLen {
				bestMatch = renderer
				bestMatchLen = len(pattern)
			}
		}
	}

	return bestMatch
}

// GetRenderersForPatterns returns all renderers that match any of the given patterns
func GetRenderersForPatterns(patterns []string) map[string]ComponentRenderer {
	globalRendererRegistry.mu.RLock()
	defer globalRendererRegistry.mu.RUnlock()

	result := make(map[string]ComponentRenderer)
	for _, pattern := range patterns {
		for registeredPattern, renderer := range globalRendererRegistry.patterns {
			if matchesPattern(pattern, registeredPattern) {
				result[registeredPattern] = renderer
			}
		}
	}
	return result
}

// InitializeDefaultRenderers registers the standard renderers with default patterns
func InitializeDefaultRenderers(
	issueService *issues.IssueService,
	projectService *projects.ProjectService,
	userService *users.UserService,
	notificationService *notifications.NotificationService,
) {
	// Create renderer instances with proper service dependencies
	issueTableRenderer := NewIssuesTableRenderer(issueService, projectService, userService)
	issueDetailRenderer := NewIssueDetailRenderer(issueService, projectService, userService)
	projectsTableRenderer := NewProjectsTableRenderer(projectService, userService)
	notificationRenderer := NewNotificationRenderer(notificationService)

	// Register renderers with their patterns
	RegisterRenderer("issues.*", issueTableRenderer)          // All issue events (table view)
	RegisterRenderer("issues.*.created", issueDetailRenderer) // Issue creation (detail view)
	RegisterRenderer("issues.*.updated", issueDetailRenderer) // Issue updates (detail view)
	RegisterRenderer("issues.*.deleted", issueDetailRenderer) // Issue deletion (detail view)

	RegisterRenderer("projects.*", projectsTableRenderer)         // All project events (table view)
	RegisterRenderer("projects.*.created", projectsTableRenderer) // Project creation
	RegisterRenderer("projects.*.updated", projectsTableRenderer) // Project updates

	RegisterRenderer("notifications.*", notificationRenderer) // All notification events
}

// matchesPattern checks if an event pattern matches a registered pattern
// This handles wildcards and pattern matching
func matchesPattern(eventPattern, registeredPattern string) bool {
	// If registered pattern has wildcards, use existing pattern matching logic
	if contains(registeredPattern, "*") {
		return MatchesEventPattern(eventPattern, []string{registeredPattern})
	}

	// Otherwise, exact match or prefix match
	return eventPattern == registeredPattern
}

// contains is a simple helper since we can't import strings here
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		(len(substr) == 1 && findChar(s, substr[0]) >= 0)
}

func findChar(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// ListRegisteredPatterns returns all registered patterns (useful for debugging)
func ListRegisteredPatterns() []string {
	globalRendererRegistry.mu.RLock()
	defer globalRendererRegistry.mu.RUnlock()

	patterns := make([]string, 0, len(globalRendererRegistry.patterns))
	for pattern := range globalRendererRegistry.patterns {
		patterns = append(patterns, pattern)
	}
	return patterns
}

// ClearRegistry clears all registered renderers (useful for testing)
func ClearRegistry() {
	globalRendererRegistry.mu.Lock()
	defer globalRendererRegistry.mu.Unlock()
	globalRendererRegistry.patterns = make(map[string]ComponentRenderer)
}
