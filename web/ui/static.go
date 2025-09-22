package ui

import (
	"embed"

	"github.com/benbjohnson/hashfs"
)

//go:embed static/*
var embedFS embed.FS

var (
	ScriptDirectoryPath = "web/ui/js"
	StylesDirectoryPath = "web/ui/styles"
	StaticDirectoryPath = "web/ui/static"
)

var StaticAssets *hashfs.FS = hashfs.NewFS(embedFS)
