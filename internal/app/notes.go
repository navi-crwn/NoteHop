package app

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

const (
	maxContent    = 1 << 20 // per note, after encryption
	maxEntry      = 64 << 10
	maxEntries    = 2000
	maxOwnerBlob  = 4096
	maxCheckIndex = 4999
	maxViews      = 1000
	maxExpiry     = 365 * 24 * time.Hour
)

// Link modes. The server enforces them; the key in the URL fragment only
// decides who can read.
const (
	modeEdit   = "edit"   // change everything
	modeRead   = "read"   // look only
	modeAppend = "append" // add lines below, never change what's there
	modeCheck  = "check"  // look, and tick checkboxes
)

func validMode(m string) bool {
	return m == modeEdit || m == modeRead || m == modeAppend || m == modeCheck
}

var (
	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,47}$`)
	keyRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`) // 32 bytes, base64url without padding
	b64Re  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

const tokenAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randToken(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(tokenAlphabet)))
	for i := range b {
		k, _ := rand.Int(rand.Reader, max)
		b[i] = tokenAlphabet[k.Int64()]
	}
	return string(b)
}

func hashHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hashMatch(secret, want string) bool {
	return secret != "" && subtle.ConstantTimeCompare([]byte(hashHex(secret)), []byte(want)) == 1
}

// validContent checks shape only: ciphertext is base64url (IV + tag at least),
// a public note is JSON with string fields.
func validContent(encrypted bool, s string, max int, fields ...string) bool {
	if s == "" || len(s) > max {
		return false
	}
	if encrypted {
		return len(s) >= 38 && b64Re.MatchString(s)
	}
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) != nil {
		return false
	}
	for _, f := range fields {
		if _, ok := m[f].(string); !ok {
			return false
		}
	}
	return true
}

// ---------- access ----------

type access struct {
	note  *db.Note
	link  *db.Link
	admin bool
	write bool // proved knowledge of the note key
}

func (x access) can(modes ...string) bool {
	if x.admin {
		return true
	}
	for _, m := range modes {
		if x.link.Mode == m {
			return true
		}
	}
	return false
}

func (a *App) access(w http.ResponseWriter, r *http.Request) (access, bool) {
	n, l, err := a.db.NoteByLink(r.PathValue("token"))
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			log.Printf("note lookup: %v", err)
		}
		apiErr(w, http.StatusNotFound, "Catatan tidak ditemukan, sudah kedaluwarsa, atau sudah dihancurkan.")
		return access{}, false
	}
	return access{
		note:  n,
		link:  l,
		admin: hashMatch(r.Header.Get("X-Admin"), n.AdminHash),
		write: hashMatch(r.Header.Get("X-Write"), n.WriteHash),
	}, true
}

// writable gates every change: right link mode, proof of the key, not frozen.
func (a *App) writable(w http.ResponseWriter, x access, modes ...string) bool {
	switch {
	case x.note.Frozen:
		apiErr(w, http.StatusForbidden, "Snapshot ini dibekukan dan tidak bisa diubah.")
	case !x.can(modes...):
		apiErr(w, http.StatusForbidden, "Link ini tidak punya izin untuk itu.")
	case !x.write:
		apiErr(w, http.StatusForbidden, "Kunci catatan tidak cocok.")
	default:
		if !x.admin {
			if e := runBeforeWrite(x.note, time.Now()); e != nil {
				apiErr(w, e.status, e.msg)
				return false
			}
		}
		return true
	}
	return false
}

func (a *App) requireAdmin(w http.ResponseWriter, x access) bool {
	if !x.admin {
		apiErr(w, http.StatusForbidden, "Hanya pemilik link admin yang bisa melakukan ini.")
	}
	return x.admin
}

// ---------- create ----------

type createReq struct {
	Encrypted bool   `json:"encrypted"`
	Password  bool   `json:"password"`
	Salt      string `json:"salt"`
	Content   string `json:"content"`
	Mode      string `json:"mode"`
	Slug      string `json:"slug"`
	ExpiresIn int64  `json:"expires_in"`
	MaxViews  int64  `json:"max_views"`
	Admin     string `json:"admin"`
	Write     string `json:"write"`
	OwnerBlob string `json:"owner_blob"`
}

func (q createReq) invalid() string {
	switch {
	case !validMode(q.Mode):
		return "Mode akses tidak dikenal."
	case !validContent(q.Encrypted, q.Content, maxContent, "t", "b"):
		return "Isi catatan kosong atau rusak."
	case q.Password && (!q.Encrypted || len(q.Salt) < 16 || len(q.Salt) > 64 || !b64Re.MatchString(q.Salt)):
		return "Password hanya bisa dipakai pada catatan terenkripsi."
	case !keyRe.MatchString(q.Admin) || !keyRe.MatchString(q.Write):
		return "Kunci tidak valid."
	case q.Slug != "" && !slugRe.MatchString(q.Slug):
		return "Slug: 3–48 karakter, huruf kecil, angka, dan tanda minus."
	case q.ExpiresIn < 0 || time.Duration(q.ExpiresIn)*time.Second > maxExpiry:
		return "Waktu kedaluwarsa tidak valid."
	case q.MaxViews < 0 || q.MaxViews > maxViews:
		return "Batas kunjungan tidak valid."
	case len(q.OwnerBlob) > maxOwnerBlob:
		return "Data pemilik terlalu besar."
	}
	return ""
}

func (a *App) apiCreate(w http.ResponseWriter, r *http.Request) {
	var q createReq
	if !readJSON(w, r, &q, maxContent+16<<10) {
		return
	}
	q.Slug = strings.ToLower(strings.TrimSpace(q.Slug))
	if msg := q.invalid(); msg != "" {
		apiErr(w, http.StatusBadRequest, msg)
		return
	}
	n := &db.Note{
		Encrypted: q.Encrypted,
		Password:  q.Password,
		Content:   q.Content,
		AdminHash: hashHex(q.Admin),
		WriteHash: hashHex(q.Write),
		MaxViews:  q.MaxViews,
	}
	if q.Password {
		n.Salt = q.Salt
	}
	if q.ExpiresIn > 0 {
		n.Expires = time.Now().Unix() + q.ExpiresIn
	}
	if u := a.currentUser(r); u != nil {
		n.OwnerID = sql.NullInt64{Int64: u.ID, Valid: true}
		n.OwnerBlob = q.OwnerBlob
	}
	link := &db.Link{Token: q.Slug, Mode: q.Mode}
	if link.Token == "" {
		link.Token = randToken(10)
	}
	if err := a.db.CreateNote(n, link); err != nil {
		if errors.Is(err, db.ErrTaken) {
			apiErr(w, http.StatusConflict, "Slug itu sudah dipakai. Coba yang lain.")
			return
		}
		log.Printf("create: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal menyimpan catatan.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": link.Token, "expires": n.Expires})
}

// ---------- read ----------

type metaResp struct {
	Mode      string        `json:"mode"`
	Encrypted bool          `json:"encrypted"`
	Password  bool          `json:"password"`
	Salt      string        `json:"salt,omitempty"`
	Frozen    bool          `json:"frozen"`
	MaxViews  int64         `json:"max_views"`
	Views     int64         `json:"views"`
	Expires   int64         `json:"expires"`
	Created   int64         `json:"created"`
	Updated   int64         `json:"updated"`
	Version   int64         `json:"version"`
	Admin     bool          `json:"admin"`
	Owned     bool          `json:"owned"`
	Links     []db.Link     `json:"links,omitempty"`
	Snapshots []db.Snapshot `json:"snapshots,omitempty"`
	Parent    string        `json:"parent,omitempty"`
	LockAt    int64         `json:"lock_at,omitempty"`
	Locked    bool          `json:"locked,omitempty"`
}

func (a *App) meta(r *http.Request, x access) metaResp {
	n := x.note
	m := metaResp{
		Mode: x.link.Mode, Encrypted: n.Encrypted, Password: n.Password, Salt: n.Salt, Frozen: n.Frozen,
		MaxViews: n.MaxViews, Views: n.Views, Expires: n.Expires, Created: n.Created, Updated: n.Updated,
		Version: n.Version, Admin: x.admin,
	}
	if u := a.currentUser(r); u != nil && n.OwnerID.Valid && n.OwnerID.Int64 == u.ID {
		m.Owned = true
	}
	fillMeta(n, &m, time.Now())
	if x.admin {
		m.Links, _ = a.db.Links(n.ID)
		m.Snapshots, _ = a.db.Snapshots(n.ID)
		m.Parent = a.db.ParentToken(n)
	}
	return m
}

// apiMeta describes a note without revealing (or spending a view on) its content.
func (a *App) apiMeta(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.meta(r, x))
}

type openResp struct {
	metaResp
	Content string          `json:"content"`
	Checks  json.RawMessage `json:"checks"`
	Entries []db.Entry      `json:"entries"`
	Burned  bool            `json:"burned"`
}

// apiOpen returns the content. Readers spend a view; once a burn-after-reading
// note hits its limit it is deleted right after this response is built.
// {"poll": true} refreshes an open editor without counting, and is refused on
// burn notes so polling can't be used to read them for free.
func (a *App) apiOpen(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok {
		return
	}
	var q struct {
		Poll bool `json:"poll"`
	}
	if r.ContentLength > 0 && !readJSON(w, r, &q, 1024) {
		return
	}
	n := x.note
	// With a password the write key doubles as a password check, so a wrong
	// guess is refused here instead of spending a burn-after-reading view.
	if n.Password && !x.write {
		apiErr(w, http.StatusForbidden, "Password salah.")
		return
	}
	if !x.admin {
		if e := runBeforeRead(n, time.Now()); e != nil {
			apiErr(w, e.status, e.msg)
			return
		}
	}
	burnable := n.MaxViews > 0 && !x.admin
	if q.Poll && burnable {
		apiErr(w, http.StatusForbidden, "Catatan sekali baca tidak bisa diperbarui otomatis.")
		return
	}
	if !x.admin && !q.Poll {
		views, err := a.db.TakeView(n)
		if err != nil {
			apiErr(w, http.StatusNotFound, "Catatan ini sudah dihancurkan.")
			return
		}
		n.Views = views
	}
	entries, err := a.db.Entries(n.ID)
	if err != nil {
		log.Printf("entries: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal membaca catatan.")
		return
	}
	resp := openResp{metaResp: a.meta(r, x), Content: n.Content, Checks: json.RawMessage(n.Checks), Entries: entries}
	if burnable && n.Views >= n.MaxViews {
		resp.Burned = true
		if err := a.db.DeleteNote(n.ID); err != nil {
			log.Printf("burn: %v", err)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// raw serves plain text for curl. Encrypted notes can't be decrypted here,
// so they explain that instead of pretending.
func (a *App) raw(w http.ResponseWriter, r *http.Request) {
	n, _, err := a.db.NoteByLink(r.PathValue("token"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found\n"))
		return
	}
	if n.Encrypted {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("This note is end-to-end encrypted; the server cannot read it.\n" +
			"Open it in a browser with the full link (including the #key part).\n"))
		return
	}
	if e := runBeforeRead(n, time.Now()); e != nil {
		w.WriteHeader(e.status)
		w.Write([]byte(e.msg + "\n"))
		return
	}
	views, err := a.db.TakeView(n)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found\n"))
		return
	}
	var body struct{ T, B string }
	_ = json.Unmarshal([]byte(n.Content), &body)
	var checks map[string]bool
	_ = json.Unmarshal([]byte(n.Checks), &checks)
	var sb strings.Builder
	sb.WriteString(applyChecks(body.B, checks))
	if !strings.HasSuffix(body.B, "\n") {
		sb.WriteByte('\n')
	}
	if entries, _ := a.db.Entries(n.ID); len(entries) > 0 {
		sb.WriteString("\n")
		for _, e := range entries {
			var en struct{ N, B string }
			_ = json.Unmarshal([]byte(e.Content), &en)
			sb.WriteString(strings.TrimRight(en.B, "\n"))
			sb.WriteByte('\n')
		}
	}
	if n.MaxViews > 0 && views >= n.MaxViews {
		_ = a.db.DeleteNote(n.ID)
	}
	w.Write([]byte(sb.String()))
}

var taskRe = regexp.MustCompile(`^(\s*(?:[-*+]|\d+[.)])\s+)\[([ xX])\]`)

// applyChecks writes checkbox overrides back into markdown. Tasks are counted
// in document order outside fenced code, the same way the browser numbers them.
func applyChecks(md string, checks map[string]bool) string {
	if len(checks) == 0 {
		return md
	}
	lines := strings.Split(md, "\n")
	fence, i := "", 0
	for k, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			if fence == "" {
				fence = t[:3]
			} else if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if m := taskRe.FindStringSubmatchIndex(line); m != nil {
			if c, ok := checks[strconv.Itoa(i)]; ok {
				mark := " "
				if c {
					mark = "x"
				}
				lines[k] = line[:m[4]] + mark + line[m[5]:]
			}
			i++
		}
	}
	return strings.Join(lines, "\n")
}

// ---------- write ----------

func (a *App) apiSave(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.writable(w, x, modeEdit) {
		return
	}
	var q struct {
		Content   string  `json:"content"`
		Version   int64   `json:"version"`
		OwnerBlob *string `json:"owner_blob"`
	}
	if !readJSON(w, r, &q, maxContent+8<<10) {
		return
	}
	if !validContent(x.note.Encrypted, q.Content, maxContent, "t", "b") {
		apiErr(w, http.StatusBadRequest, "Isi catatan kosong atau rusak.")
		return
	}
	if q.OwnerBlob != nil {
		u := a.currentUser(r)
		if !x.admin || u == nil || !x.note.OwnerID.Valid || x.note.OwnerID.Int64 != u.ID || len(*q.OwnerBlob) > maxOwnerBlob {
			q.OwnerBlob = nil
		}
	}
	v, err := a.db.UpdateContent(x.note.ID, q.Version, q.Content, q.OwnerBlob)
	if errors.Is(err, db.ErrConflict) {
		apiErr(w, http.StatusConflict, "Catatan sudah diubah orang lain.")
		return
	}
	if err != nil {
		log.Printf("save: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal menyimpan.")
		return
	}
	runAfterUpdate(x.note, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{"version": v})
}

func (a *App) apiAddEntry(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.writable(w, x, modeEdit, modeAppend) {
		return
	}
	var q struct {
		Content string `json:"content"`
	}
	if !readJSON(w, r, &q, maxEntry+1024) {
		return
	}
	if !validContent(x.note.Encrypted, q.Content, maxEntry, "b") {
		apiErr(w, http.StatusBadRequest, "Tulisan kosong atau terlalu panjang.")
		return
	}
	if c, _ := a.db.CountEntries(x.note.ID); c >= maxEntries {
		apiErr(w, http.StatusForbidden, "Catatan ini sudah penuh.")
		return
	}
	e, err := a.db.AddEntry(x.note.ID, q.Content)
	if err != nil {
		log.Printf("entry: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal menambahkan.")
		return
	}
	runAfterUpdate(x.note, time.Now())
	writeJSON(w, http.StatusCreated, e)
}

func (a *App) apiDeleteEntry(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := a.db.DeleteEntry(x.note.ID, id); err != nil {
		apiErr(w, http.StatusNotFound, "Baris tidak ditemukan.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) apiCheck(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.writable(w, x, modeEdit, modeCheck) {
		return
	}
	var q struct {
		Index   int  `json:"index"`
		Checked bool `json:"checked"`
	}
	if !readJSON(w, r, &q, 256) {
		return
	}
	if q.Index < 0 || q.Index > maxCheckIndex {
		apiErr(w, http.StatusBadRequest, "Checkbox tidak valid.")
		return
	}
	checks, err := a.db.SetCheck(x.note.ID, q.Index, q.Checked)
	if err != nil {
		log.Printf("check: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal menyimpan centang.")
		return
	}
	runAfterUpdate(x.note, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{"checks": checks})
}

// ---------- admin ----------

func (a *App) apiFreeze(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	if x.note.Frozen {
		apiErr(w, http.StatusBadRequest, "Ini sudah snapshot.")
		return
	}
	l, err := a.db.Freeze(x.note, randToken(12))
	if err != nil {
		log.Printf("freeze: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal membekukan.")
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (a *App) apiAddLink(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	var q struct {
		Mode string `json:"mode"`
		Slug string `json:"slug"`
	}
	if !readJSON(w, r, &q, 512) {
		return
	}
	q.Slug = strings.ToLower(strings.TrimSpace(q.Slug))
	switch {
	case !validMode(q.Mode):
		apiErr(w, http.StatusBadRequest, "Mode akses tidak dikenal.")
		return
	case x.note.Frozen && q.Mode != modeRead:
		apiErr(w, http.StatusBadRequest, "Snapshot hanya bisa dibagikan sebagai baca saja.")
		return
	case q.Slug != "" && !slugRe.MatchString(q.Slug):
		apiErr(w, http.StatusBadRequest, "Slug: 3–48 karakter, huruf kecil, angka, dan tanda minus.")
		return
	}
	l := &db.Link{Token: q.Slug, NoteID: x.note.ID, Mode: q.Mode}
	if l.Token == "" {
		l.Token = randToken(10)
	}
	if err := a.db.AddLink(l); err != nil {
		if errors.Is(err, db.ErrTaken) {
			apiErr(w, http.StatusConflict, "Slug itu sudah dipakai. Coba yang lain.")
			return
		}
		log.Printf("link: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal membuat link.")
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (a *App) apiDeleteLink(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	switch err := a.db.DeleteLink(x.note.ID, r.PathValue("link")); {
	case errors.Is(err, db.ErrLastLink):
		apiErr(w, http.StatusBadRequest, "Catatan butuh minimal satu link. Hapus catatannya saja.")
	case errors.Is(err, db.ErrNotFound):
		apiErr(w, http.StatusNotFound, "Link tidak ditemukan.")
	case err != nil:
		log.Printf("unlink: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal menghapus link.")
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (a *App) apiDelete(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok {
		return
	}
	owner := false
	if u := a.currentUser(r); u != nil && x.note.OwnerID.Valid && x.note.OwnerID.Int64 == u.ID {
		owner = true
	}
	if !x.admin && !owner {
		apiErr(w, http.StatusForbidden, "Hanya pemilik yang bisa menghapus catatan.")
		return
	}
	if err := a.db.DeleteNote(x.note.ID); err != nil {
		log.Printf("delete: %v", err)
		apiErr(w, http.StatusInternalServerError, "Gagal menghapus.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
