package app

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/navi-crwn/NoteHop/internal/i18n"
)

// https://hel.ink serves the HOP family fonts and app marks: styles, fonts and
// images only. Scripts stay 'self', so notes are never exposed to another origin.
const csp = "default-src 'self'; script-src 'self'; style-src 'self' https://hel.ink; img-src 'self' data: blob: https://hel.ink; media-src 'self' blob:; " +
	"connect-src 'self'; font-src 'self' https://hel.ink; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// aiOrigins are the AI providers a reader can bring their own key for (ai.js).
// The browser calls them directly; NoteHop never sees the text or the key.
var aiOrigins = []string{"https://api.anthropic.com", "https://api.openai.com", "https://generativelanguage.googleapis.com",
	"https://openrouter.ai", "http://localhost:11434", "http://127.0.0.1:11434"}

// cspFor opens connect-src to the AI providers on the pages with an editor
// only, and only when the operator allows API keys (NOTEHOP_AI=on).
func (a *App) cspFor(path string) string {
	if a.cfg.aiMode() != "on" || !(path == "/new" || strings.HasPrefix(path, "/n/") || shortName(strings.TrimPrefix(path, "/"))) {
		return csp
	}
	extra := strings.Join(append(append([]string{}, aiOrigins...), a.cfg.AIOrigins...), " ")
	return strings.Replace(csp, "connect-src 'self'", "connect-src 'self' "+extra, 1)
}

// secure adds security headers, rejects cross-site writes and rate-limits the API.
func (a *App) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", a.cspFor(r.URL.Path))
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		// A note, basic or encrypted, never belongs in a search engine.
		if servesNote(r.URL.Path) {
			h.Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		}
		if a.cfg.SecureCookies {
			h.Set("Strict-Transport-Security", "max-age=63072000")
		}
		// Public text and mock APIs are free to read, but not a free CDN: one
		// address gets 120 requests a minute across /raw and /mock.
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && (strings.HasPrefix(r.URL.Path, "/raw/") || strings.HasPrefix(r.URL.Path, "/mock/")) &&
			!a.limit.allow("g"+a.clientIP(r), 120) {
			w.Header().Set("Retry-After", "60")
			apiErr(w, http.StatusTooManyRequests, "err.rate_limited")
			return
		}
		// The control center's calls are signed, not cookie-authenticated,
		// so the cross-site check and the per-address limits don't apply.
		if strings.HasPrefix(r.URL.Path, "/hop/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !sameOrigin(r) {
				apiErr(w, http.StatusForbidden, "err.cross_site")
				return
			}
			bucket, rate := "w", 120
			switch {
			case r.URL.Path == "/api/notes":
				bucket, rate = "c", 30
			case r.URL.Path == "/api/files" || (strings.HasPrefix(r.URL.Path, "/api/n/") && strings.HasSuffix(r.URL.Path, "/files")):
				bucket, rate = "f", 20
			case strings.HasPrefix(r.URL.Path, "/in/"):
				bucket, rate = "i", 60
			case r.URL.Path == "/api/login" || r.URL.Path == "/api/register" || r.URL.Path == "/api/vault":
				bucket, rate = "a", 15
			case strings.HasPrefix(r.URL.Path, "/api/recover") || r.URL.Path == "/api/me/secret" || r.URL.Path == "/api/me/recovery":
				bucket, rate = "k", 20 // recovery.go adds a limit per account
			case strings.HasSuffix(r.URL.Path, "/report"):
				bucket, rate = "r", 10 // report.go adds 5 per hour per address
			}
			if !a.limit.allow(bucket+a.clientIP(r), rate) {
				w.Header().Set("Retry-After", "60")
				apiErr(w, http.StatusTooManyRequests, "err.rate_limited")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Fetch Metadata remains available when no-referrer makes a form's Origin
// opaque. The standard check prefers that browser-controlled metadata and
// falls back to Origin for older clients.
func sameOrigin(r *http.Request) bool {
	return http.NewCrossOriginProtection().Check(r) == nil
}

func (a *App) clientIP(r *http.Request) string {
	if a.cfg.TrustProxy {
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			return strings.TrimSpace(strings.Split(f, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limiter is a fixed one-minute window per key. Good enough for a single node.
type limiter struct {
	mu sync.Mutex
	m  map[string]*window
}

type window struct {
	start time.Time
	n     int
}

func newLimiter() *limiter { return &limiter{m: map[string]*window{}} }

func (l *limiter) allow(key string, perMin int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.m[key]
	if w == nil || time.Since(w.start) > time.Minute {
		l.m[key] = &window{start: time.Now(), n: 1}
		return true
	}
	w.n++
	return w.n <= perMin
}

func (l *limiter) gc() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, w := range l.m {
		if time.Since(w.start) > time.Minute {
			delete(l.m, k)
		}
	}
}

// ---------- JSON helpers ----------

func readJSON(w http.ResponseWriter, r *http.Request, v any, max int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			apiErr(w, http.StatusRequestEntityTooLarge, "err.too_large")
		} else {
			apiErr(w, http.StatusBadRequest, "err.bad_request")
		}
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiErr answers with a catalog key ("code"), its English text ("error",
// for curl and logs) and the values for its placeholders ("vars"); the
// browser shows the text in the reader's language. A time.Time value is
// sent as unix seconds so the browser can show it in local time.
func apiErr(w http.ResponseWriter, status int, key string, kv ...any) {
	resp := map[string]any{"error": i18n.T(i18n.Default, key, kv...), "code": key}
	if len(kv) > 1 {
		vars := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			name, _ := kv[i].(string)
			if t, ok := kv[i+1].(time.Time); ok {
				vars[name] = t.Unix()
			} else {
				vars[name] = kv[i+1]
			}
		}
		resp["vars"] = vars
	}
	writeJSON(w, status, resp)
}

// servesNote reports whether a request path leads to a note or to something that
// serves note data: /TOKEN or /link-name, the older /n/TOKEN, the plain-text
// and mock endpoints, the API and the inbox addresses, and the profile pages
// that list notes. Site pages are not notes.
func servesNote(p string) bool {
	for _, prefix := range []string{"/n/", "/raw/", "/mock/", "/api/", "/in/", "/gate/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	if strings.HasPrefix(p, "/@") { // a profile lists notes
		return true
	}
	seg := strings.TrimPrefix(p, "/")
	return shortName(seg)
}
