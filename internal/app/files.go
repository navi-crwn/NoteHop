package app

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Attachments: the browser encrypts each file with its own random key and
// uploads only the ciphertext. The key travels inside the note text
// (nh-file:ID#KEY), so whoever can read the note can open the file, and the
// server can't. Files are temporary: they disappear after a day.
const fileTTL = 24 * time.Hour

var fileIDRe = regexp.MustCompile(`^[A-Za-z0-9]{20}$`)

func (a *App) apiUpload(w http.ResponseWriter, r *http.Request) {
	if a.cfg.FileMB <= 0 {
		apiErr(w, http.StatusForbidden, "err.files_off")
		return
	}
	scope := ""
	if r.PathValue("token") != "" {
		x, ok := a.access(w, r)
		if !ok || !a.writable(w, x, modeEdit, modeAppend) {
			return
		}
		scope = "note:" + strconv.FormatInt(x.note.ID, 10)
	} else {
		draft := r.Header.Get("X-Upload-Draft")
		if !keyRe.MatchString(draft) {
			apiErr(w, http.StatusBadRequest, "err.files_note")
			return
		}
		scope = "draft:" + hashHex(draft)
	}
	a.uploadMu.Lock()
	defer a.uploadMu.Unlock()
	if !a.storageAvailable(int64(a.cfg.FileMB) << 20) {
		w.Header().Set("Retry-After", "60")
		apiErr(w, http.StatusInsufficientStorage, "err.files_storage")
		return
	}
	// AES-GCM adds 28 bytes; the encrypted header with name and type a few hundred.
	max := int64(a.cfg.FileMB)<<20 + 4096
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			apiErr(w, http.StatusRequestEntityTooLarge, "err.file_too_large", "n", a.cfg.FileMB)
		} else {
			apiErr(w, http.StatusBadRequest, "err.bad_request")
		}
		return
	}
	if len(data) < 29 {
		apiErr(w, http.StatusBadRequest, "err.bad_request")
		return
	}
	// One address can't fill the disk for everyone: a daily budget per address.
	if !a.quota.take(a.clientIP(r), int64(len(data)), int64(a.cfg.FilesPerIPMB)<<20, time.Now()) {
		w.Header().Set("Retry-After", "3600")
		apiErr(w, http.StatusTooManyRequests, "err.files_quota", "n", a.cfg.FilesPerIPMB)
		return
	}
	now := time.Now()
	f := db.File{ID: randToken(20), Size: int64(len(data)), Data: data, Created: now.Unix(), Expires: now.Add(fileTTL).Unix(), Scope: scope}
	ok, err := a.db.AddFile(f, int64(a.cfg.FilesTotalMB)<<20)
	if errors.Is(err, db.ErrNoteFilesFull) {
		apiErr(w, http.StatusRequestEntityTooLarge, "err.files_note_full")
		return
	}
	if errors.Is(err, db.ErrTaken) {
		apiErr(w, http.StatusConflict, "err.files_note")
		return
	}
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	if !ok {
		w.Header().Set("Retry-After", "60")
		apiErr(w, http.StatusInsufficientStorage, "err.files_full")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": f.ID, "expires": f.Expires})
}

func (a *App) apiFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !fileIDRe.MatchString(id) {
		apiErr(w, http.StatusNotFound, "err.file_gone")
		return
	}
	f, err := a.db.GetFile(id)
	if err != nil {
		apiErr(w, http.StatusNotFound, "err.file_gone")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Length", strconv.FormatInt(f.Size, 10))
	h.Set("Cache-Control", "private, max-age="+strconv.FormatInt(max(f.Expires-time.Now().Unix(), 0), 10)+", immutable")
	h.Set("X-Expires", strconv.FormatInt(f.Expires, 10))
	w.Write(f.Data)
}

// dayQuota counts the bytes each address uploaded in the last 24 hours (a
// fixed window from its first upload). In memory: a restart forgives.
type dayQuota struct {
	mu sync.Mutex
	m  map[string]*dayUse
}

type dayUse struct {
	start time.Time
	bytes int64
}

// take adds n bytes for key and reports whether it stays within max (0 = no limit).
func (q *dayQuota) take(key string, n, max int64, now time.Time) bool {
	if max <= 0 {
		return true
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for k, u := range q.m {
		if now.Sub(u.start) > 24*time.Hour {
			delete(q.m, k)
		}
	}
	u := q.m[key]
	if u == nil {
		u = &dayUse{start: now}
		q.m[key] = u
	}
	if u.bytes+n > max {
		return false
	}
	u.bytes += n
	return true
}
