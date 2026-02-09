package server

import (
	"embed"
	"io/fs"
)

// staticAssets holds the exported Studio frontend (copied into internal/server/static).
//
//go:embed all:static
var staticAssets embed.FS

func staticFS() (fs.FS, error) {
	return fs.Sub(staticAssets, "static")
}
