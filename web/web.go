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

// HopAssets is a copy of the HOP asset manifest published at
// https://hel.ink/hop-assets/apps.json (fonts, app marks and their SRI hashes).
// Refresh it with scripts/sync-hop-assets.sh.
//
//go:embed hopassets/apps.json
var HopAssets []byte

//go:embed changelog.json
var Changelog []byte
