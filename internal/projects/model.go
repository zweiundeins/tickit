package projects

import "time"

// Project represents a project entity
// Extendable for more fields if needed
// ID is a string (UUID), slug is a unique string identifier
// created_at and updated_at are timestamps

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Archived    bool      `json:"archived"`
}

// PatchProjectInput is used for type-safe partial updates to a Project
// Only non-nil fields will be updated
// ID is required
type PatchProjectInput struct {
	ID          string
	Name        *string
	Slug        *string
	Description *string
	Archived    *bool
}
