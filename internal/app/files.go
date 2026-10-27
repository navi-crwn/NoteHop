package app

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
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
	now := time.Now()
	f := db.File{ID: randToken(20), Size: int64(len(data)), Data: data, Created: now.Unix(), Expires: now.Add(fileTTL).Unix()}
	ok, err := a.db.AddFile(f, int64(a.cfg.FilesTotalMB)<<20)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	if !ok {
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
