// Package web embeds the dashboard static files into the binary.
//
// Why: embed.FS requires the //go:embed directive to reference files
// relative to the Go source file. The web/ directory lives at the project
// root, so this package sits right next to the HTML/CSS/JS files and
// exports the embedded filesystem for the API server to serve.
package web

import "embed"

// Assets contains all dashboard files (index.html, styles.css, app.js).
// The Go compiler bakes these into the binary at compile time.
//
//go:embed index.html styles.css app.js
var Assets embed.FS
