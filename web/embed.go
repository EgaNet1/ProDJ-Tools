// Package web holds the interface. The files are compiled into the program,
// so ProDJ Tools ships as a single executable.
package web

import "embed"

//go:embed index.html style.css app.js
var Files embed.FS
