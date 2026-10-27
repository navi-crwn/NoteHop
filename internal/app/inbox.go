package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Inbox (webhook-to-note): other services POST text to /in/<token> and it
// becomes an entry of the note. The server does see that text while the
// request is in flight — it comes from outside, in the clear — but stores
// only ciphertext: each entry is sealed to the note's public key (P-256
// ECDH + HKDF-SHA256 + AES-256-GCM, a fresh key per entry). The private key
// was made in the browser and is kept sealed with the note key.
//
// Entry content: "x1." + base64url(ephemeral public key) + "." + base64url(iv ‖ ciphertext)

const inboxMax = 64 << 10

var b64u = base64.RawURLEncoding

func (a *App) apiInboxOn(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	if x.note.Frozen {
		apiErr(w, http.StatusForbidden, "err.frozen")
		return
	}
	var q struct {
		Pub  string `json:"pub"`
		Priv string `json:"priv"`
	}
	if !readJSON(w, r, &q, 8192) {
		return
	}
	raw, err := b64u.DecodeString(q.Pub)
	if err != nil || len(q.Priv) < 40 || len(q.Priv) > 4096 {
		apiErr(w, http.StatusBadRequest, "err.bad_request")
		return
	}
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		apiErr(w, http.StatusBadRequest, "err.bad_request")
		return
	}
	in := db.Inbox{Token: randToken(24), NoteID: x.note.ID, Pub: q.Pub, Priv: q.Priv}
	if err := a.db.SetInbox(in); err != nil {
		apiErr(w, http.StatusInternalServerError, "err.config_failed")
		return
	}
	writeJSON(w, http.StatusOK, a.meta(r, x))
}

func (a *App) apiInboxOff(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	if err := a.db.DeleteInbox(x.note.ID); err != nil {
		apiErr(w, http.StatusInternalServerError, "err.config_failed")
		return
	}
	writeJSON(w, http.StatusOK, a.meta(r, x))
}

// sealTo encrypts plain for the holder of the private key behind pub.
func sealTo(pub string, plain []byte) (string, error) {
	raw, err := b64u.DecodeString(pub)
	if err != nil {
		return "", err
	}
	them, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return "", err
	}
	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	shared, err := eph.ECDH(them)
	if err != nil {
		return "", err
	}
	key, err := hkdf.Key(sha256.New, shared, nil, "notehop/inbox", 32)
	if err != nil {
		return "", err
	}
	blk, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(blk)
	iv := make([]byte, 12)
	rand.Read(iv)
	return "x1." + b64u.EncodeToString(eph.PublicKey().Bytes()) + "." + b64u.EncodeToString(gcm.Seal(iv, iv, plain, nil)), nil
}

var errInboxGone = errors.New("inbox not found")

// deliverInbox seals text (from: who sent it) into an entry of the inbox's note.
func (a *App) deliverInbox(token, from, text string) error {
	in, err := a.db.InboxByToken(token)
	if err != nil {
		return errInboxGone
	}
	n, err := a.db.NoteByID(in.NoteID)
	if err != nil || n.Frozen {
		return errInboxGone
	}
	if c, _ := a.db.CountEntries(n.ID); c >= maxEntries {
		return errors.New("full")
	}
	content, err := sealTo(in.Pub, mustJSON(map[string]string{"n": from, "b": text}))
	if err != nil {
		return err
	}
	if _, err := a.db.AddEntry(n.ID, content); err != nil {
		return err
	}
	a.runAfterUpdate(n, time.Now(), "entry")
	return nil
}

// apiInbox accepts plain text, or JSON with the text in "text", "content",
// "message" or "body" and the sender in "from", "name" or "title".
func (a *App) apiInbox(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, inboxMax))
	if err != nil {
		apiErr(w, http.StatusRequestEntityTooLarge, "err.too_large")
		return
	}
	text, from := string(raw), ""
	if strings.HasPrefix(strings.TrimSpace(r.Header.Get("Content-Type")), "application/json") {
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			text = firstString(m, "text", "content", "message", "body")
			from = firstString(m, "from", "name", "title")
			if text == "" {
				pretty, _ := json.MarshalIndent(m, "", "  ")
				text = "```json\n" + string(pretty) + "\n```"
			}
		}
	}
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) {
		apiErr(w, http.StatusBadRequest, "err.bad_request")
		return
	}
	if from == "" {
		from = "webhook"
	}
	if err := a.deliverInbox(r.PathValue("inbox"), clipRunes(from, 80), text); err != nil {
		if errors.Is(err, errInboxGone) {
			apiErr(w, http.StatusNotFound, "err.not_found")
			return
		}
		apiErr(w, http.StatusForbidden, "err.full")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
