//go:build dev
// +build dev

package main

import (
	"net/http"
	"os"
)

func static() http.Handler {
	// slog.Info("static assets are being served from web/ui/static/")
	// return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 	// Aggressive caching: 1 year immutable
	// 	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	// 	hashFS.FileServer(ui.StaticAssets).ServeHTTP(w, r)
	// })

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.FileServerFS(os.DirFS("web/ui")).ServeHTTP(w, r)
	})
}
