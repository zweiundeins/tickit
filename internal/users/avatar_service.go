package users

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AvatarService handles avatar file operations
type AvatarService struct {
	uploadDir string
}

// NewAvatarService creates a new AvatarService
func NewAvatarService(uploadDir string) *AvatarService {
	return &AvatarService{
		uploadDir: uploadDir,
	}
}

// CleanupUserAvatar removes the user's avatar file from disk
// This is called when a user is deactivated or changes their avatar
func (s *AvatarService) CleanupUserAvatar(avatarURL string) error {
	if avatarURL == "" {
		return nil // No avatar to clean up
	}

	// Only clean up local upload files, not external URLs
	if !strings.HasPrefix(avatarURL, "/uploads/") {
		return nil // External URL, nothing to clean up
	}

	// Extract filename from URL
	filename := strings.TrimPrefix(avatarURL, "/uploads/")
	if filename == "" {
		return nil
	}

	// Construct full file path
	filePath := filepath.Join(s.uploadDir, filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // File doesn't exist, nothing to clean up
	}

	// Remove the file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove avatar file %s: %w", filePath, err)
	}

	return nil
}

// GetAvatarPath returns the full file system path for an avatar URL
func (s *AvatarService) GetAvatarPath(avatarURL string) string {
	if !strings.HasPrefix(avatarURL, "/uploads/") {
		return ""
	}

	filename := strings.TrimPrefix(avatarURL, "/uploads/")
	return filepath.Join(s.uploadDir, filename)
}

// IsLocalAvatar checks if an avatar URL is a local upload
func (s *AvatarService) IsLocalAvatar(avatarURL string) bool {
	return strings.HasPrefix(avatarURL, "/uploads/")
}

// ValidateAvatarFile checks if an avatar file exists and is valid
func (s *AvatarService) ValidateAvatarFile(avatarURL string) error {
	if !s.IsLocalAvatar(avatarURL) {
		return nil // External URLs are not validated
	}

	filePath := s.GetAvatarPath(avatarURL)
	if filePath == "" {
		return fmt.Errorf("invalid avatar URL")
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("avatar file does not exist")
	}
	if err != nil {
		return fmt.Errorf("failed to check avatar file: %w", err)
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return fmt.Errorf("avatar path is not a regular file")
	}

	// Check file size (max 5MB)
	if info.Size() > 5<<20 {
		return fmt.Errorf("avatar file is too large")
	}

	return nil
}
