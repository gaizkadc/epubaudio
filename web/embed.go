package web

import "embed"

// DistFS contains the compiled frontend assets.
//
//go:embed all:dist
var DistFS embed.FS

