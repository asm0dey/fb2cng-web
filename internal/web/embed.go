// Package web holds the embedded static frontend.
package web

import "embed"

//go:embed index.html app.js
var FS embed.FS
