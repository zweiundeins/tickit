package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/bootstrap"
)

// RegisterUploadRoutes registers upload/static file routes.
func RegisterUploadRoutes(router chi.Router, deps *bootstrap.CQRSDeps) {
	router.Post("/upload", HandleUpload())
	router.Handle("/uploads/*", HandleUploads())
}
