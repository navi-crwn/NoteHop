package app

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/navi-crwn/NoteHop/internal/i18n"
)

const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; " +
	"connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// secure adds security headers, rejects cross-site writes and rate-limits the API.
func (a *App) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if a.cfg.SecureCookies {
			h.Set("Strict-Transport-Security", "max-age=63072000")
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
			case r.URL.Path == "/api/files":
				bucket, rate = "f", 20
			case r.URL.Path == "/api/login" || r.URL.Path == "/api/register" || r.URL.Path == "/api/vault":
				bucket, rate = "a", 15
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

// sameOrigin accepts requests whose Origin (when sent) matches the Host.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
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
