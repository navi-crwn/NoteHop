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
)

const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; " +
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
				apiErr(w, http.StatusForbidden, "Permintaan lintas situs ditolak.")
				return
			}
			bucket, rate := "w", 120
			switch {
			case r.URL.Path == "/api/notes":
				bucket, rate = "c", 30
			case r.URL.Path == "/api/login" || r.URL.Path == "/api/register":
				bucket, rate = "a", 15
			}
			if !a.limit.allow(bucket+a.clientIP(r), rate) {
				w.Header().Set("Retry-After", "60")
				apiErr(w, http.StatusTooManyRequests, "Terlalu banyak permintaan. Coba lagi sebentar lagi.")
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
			apiErr(w, http.StatusRequestEntityTooLarge, "Catatan terlalu besar.")
		} else {
			apiErr(w, http.StatusBadRequest, "Permintaan tidak valid.")
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

func apiErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
