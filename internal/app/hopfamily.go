package app

import (
	"encoding/json"
	"sync"

	"github.com/navi-crwn/NoteHop/web"
)

// familyAsset is one file in the HOP asset manifest, with its SRI hash.
type familyAsset struct {
	URL       string `json:"url"`
	Integrity string `json:"integrity"`
}

// familyApp is one app of the HOP family as the manifest lists it.
type familyApp struct {
	Slug    string                 `json:"slug"`
	Name    string                 `json:"name"`
	Tagline string                 `json:"tagline"`
	URL     string                 `json:"url"`
	Public  bool                   `json:"public"`
	Icons   map[string]familyAsset `json:"icons"`
}

type hopManifest struct {
	Assets map[string]familyAsset `json:"assets"`
	Apps   []familyApp            `json:"apps"`
}

var hopManifestOnce = sync.OnceValue(func() hopManifest {
	var m hopManifest
	_ = json.Unmarshal(web.HopAssets, &m) // a broken copy only drops the fonts and the family list
	return m
})

// hopFonts is the family stylesheet (Inter, Instrument Serif, JetBrains Mono) on hel.ink.
func hopFonts() *familyAsset {
	if a, ok := hopManifestOnce().Assets["fonts_css"]; ok && a.URL != "" {
		return &a
	}
	return nil
}

// hopUIAssets are the family's shared dialogs and reminder cards (hop-ui.js
// and its stylesheet, hop-assets v5 on hel.ink).
type hopUIAssets struct{ CSS, JS familyAsset }

// hopUI is nil until the bundled manifest lists both files with the script's
// SRI hash; the pages then keep the browser's own dialogs (app.js falls back).
func hopUI() *hopUIAssets {
	m := hopManifestOnce().Assets
	css, js := m["ui_css"], m["ui_js"]
	if css.URL == "" || js.URL == "" || js.Integrity == "" {
		return nil
	}
	return &hopUIAssets{CSS: css, JS: js}
}

// hopFamily lists the other public HOP apps, in family order.
func hopFamily(current string) []familyApp {
	var out []familyApp
	for _, a := range hopManifestOnce().Apps {
		if a.Public && a.Slug != current {
			out = append(out, a)
		}
	}
	return out
}
