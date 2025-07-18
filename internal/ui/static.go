package ui

import (
	"embed"

	"github.com/benbjohnson/hashfs"
)

//go:embed static/*
var embedFS embed.FS

var StaticAssets *hashfs.FS = hashfs.NewFS(embedFS)
