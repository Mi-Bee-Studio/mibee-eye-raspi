package web

import "embed"

// staticFS holds the embedded static assets for the web UI.
// The frontend files (index.html, app.js, style.css) are managed by the
// frontend agent — this package only embeds them.
//
//go:embed static/index.html static/app.js static/style.css
var staticFS embed.FS
