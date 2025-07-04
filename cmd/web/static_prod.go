//go:build !dev
// +build !dev

package main

import (
	"log/slog"
	"net/http"

	hashFS "github.com/benbjohnson/hashfs"
	"github.com/mbolli/tickit/internal/ui"
)

func static() http.Handler {
	slog.Debug("static assets are embedded")
	return hashFS.FileServer(ui.StaticAssets)
}
