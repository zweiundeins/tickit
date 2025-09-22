//go:build !dev
// +build !dev

package main

import (
	"net/http"

	hashFS "github.com/benbjohnson/hashfs"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/web/ui"
)

func static() http.Handler {
	logger.Debug().Msg("static assets are embedded")
	return hashFS.FileServer(ui.StaticAssets)
}
