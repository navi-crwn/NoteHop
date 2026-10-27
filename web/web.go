// Package web holds the HTML templates and static files, compiled into the
// binary so a NoteHop install is a single file.
package web

import "embed"

//go:embed templates
var Templates embed.FS

//go:embed static
var Static embed.FS

// I18n holds one JSON catalog per language (key → text), shared by the
// server and the browser scripts.
//
//go:embed i18n
var I18n embed.FS
