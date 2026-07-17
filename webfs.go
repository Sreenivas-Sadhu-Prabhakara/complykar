// Package complykar embeds the static web UI served by cmd/server.
package complykar

import "embed"

// WebFS holds the embedded web assets (index.html, styles.css, app.js).
//
//go:embed web
var WebFS embed.FS
