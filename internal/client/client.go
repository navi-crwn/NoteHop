// Package client talks to a NoteHop server the way the browser does: notes
// are encrypted here, before anything is sent, with the same scheme as
// web/static/js/crypto.js. Used by "notehop new" and "notehop get".
package client

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const noteIter = 310000 // must match NOTE_ITER in crypto.js

var b64 = base64.RawURLEncoding

// Keys are what a note key (and optional password) turn into.
type Keys struct {
	AES   []byte // AES-256-GCM key for the content
	Write string // proof of the key, sent with every change
}

// Derive mirrors NHC.noteKeys. Supports both legacy 256-bit (32 bytes) and 128-bit (16 bytes) keys.
func Derive(k, password, salt string) (Keys, error) {
	key, err := b64.DecodeString(strings.TrimRight(k, "="))
	if err != nil || (len(key) != 32 && len(key) != 16) {
		return Keys{}, errors.New("bad note key")
	}
	secret := key
	if password != "" {
		s, err := b64.DecodeString(strings.TrimRight(salt, "="))
		if err != nil {
			return Keys{}, errors.New("bad salt")
		}
		secret, err = pbkdf2.Key(sha256.New, password, append(append([]byte{}, key...), s...), noteIter, 32)
		if err != nil {
			return Keys{}, err
		}
	}
	return Keys{AES: mac(secret, "notehop/enc"), Write: b64.EncodeToString(mac(secret, "notehop/write"))}, nil
}

func mac(key []byte, label string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(label))
	return h.Sum(nil)
}

// Seal is iv ‖ AES-GCM(ciphertext), base64url, like seal() in crypto.js.
func Seal(key []byte, plain []byte) (string, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return "", err
	}
	iv := make([]byte, 12)
	rand.Read(iv)
	return b64.EncodeToString(gcm.Seal(iv, iv, plain, nil)), nil
}

func Open(key []byte, sealed string) ([]byte, error) {
	raw, err := b64.DecodeString(strings.TrimRight(strings.Join(strings.Fields(sealed), ""), "="))
	if err != nil || len(raw) < 28 {
		return nil, errors.New("bad ciphertext")
	}
	gcm, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, raw[:12], raw[12:], nil)
}

func gcmFor(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func NewKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return b64.EncodeToString(b)
}

func NewKey128() string {
	b := make([]byte, 16)
	rand.Read(b)
	return b64.EncodeToString(b)
}

func newSalt() string {
	b := make([]byte, 16)
	rand.Read(b)
	return b64.EncodeToString(b)
}

// Note is the decrypted content, as the browser stores it.
type Note struct {
	Title string `json:"t"`
	Body  string `json:"b"`
}

type Options struct {
	Mode      string // read, edit, append, check
	Password  string
	Public    bool // no encryption: readable by the server, /raw and curl
	ExpiresIn int  // seconds, 0 = never
	MaxViews  int  // burn after this many views
	Slug      string
}

// Created holds the links of a new note.
type Created struct {
	Token string
	Share string // for readers (no admin token)
	Admin string // keep private: edit, share, delete
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func call(method, u string, body any, hdr map[string]string, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "notehop-cli")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return fmt.Errorf("%s", e.Error)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Create encrypts n locally and stores it on the server at base.
func Create(base string, n Note, o Options) (Created, error) {
	base = strings.TrimRight(base, "/")
	if o.Mode == "" {
		o.Mode = "read"
	}
	if o.Public && o.Password != "" {
		return Created{}, errors.New("a public note can't have a password")
	}
	k, admin, salt := NewKey(), NewKey(), ""
	if o.Password != "" {
		salt = newSalt()
	}
	keys, err := Derive(k, o.Password, salt)
	if err != nil {
		return Created{}, err
	}
	plain, _ := json.Marshal(n)
	content := string(plain)
	if !o.Public {
		if content, err = Seal(keys.AES, plain); err != nil {
			return Created{}, err
		}
	}
	var res struct {
		Token string `json:"token"`
		Link  string `json:"link"` // the reader link, short on a share domain
	}
	err = call("POST", base+"/api/notes", map[string]any{
		"encrypted": !o.Public, "password": o.Password != "", "salt": salt, "content": content, "mode": o.Mode,
		"slug": o.Slug, "expires_in": o.ExpiresIn, "max_views": o.MaxViews, "admin": admin, "write": keys.Write,
	}, nil, &res)
	if err != nil {
		return Created{}, err
	}
	link := base + "/" + url.PathEscape(res.Token)
	share := res.Link
	if share == "" {
		share = link
	}
	return Created{Token: res.Token, Share: share + "#k=" + k, Admin: link + "#k=" + k + "&a=" + admin}, nil
}

// Get opens a note link (counting a view, like a browser would) and
// decrypts it here.
func Get(link, password string) (Note, error) {
	u, err := url.Parse(link)
	if err != nil {
		return Note{}, err
	}
	frag, _ := url.ParseQuery(u.Fragment)
	token := Token(link)
	if u.Host == "" || token == "" || strings.Contains(token, "/") {
		return Note{}, errors.New("not a note link (https://site.example/TOKEN#k=…)")
	}
	base := u.Scheme + "://" + u.Host
	api := base + "/api/n/" + url.PathEscape(token)
	var meta struct {
		Encrypted bool   `json:"encrypted"`
		Password  bool   `json:"password"`
		Salt      string `json:"salt"`
	}
	if err := call("GET", api, nil, nil, &meta); err != nil {
		return Note{}, err
	}
	if meta.Password && password == "" {
		return Note{}, errors.New("this note has a password (use -password)")
	}
	hdr := map[string]string{}
	var keys Keys
	if k := frag.Get("k"); k != "" {
		if keys, err = Derive(k, password, meta.Salt); err != nil {
			return Note{}, err
		}
		hdr["X-Write"] = keys.Write
	} else if meta.Encrypted {
		return Note{}, errors.New("the link has no key (#k=…)")
	}
	var data struct {
		Content string `json:"content"`
	}
	if err := call("POST", api+"/open", map[string]any{}, hdr, &data); err != nil {
		return Note{}, err
	}
	plain := []byte(data.Content)
	if meta.Encrypted {
		if plain, err = Open(keys.AES, data.Content); err != nil {
			return Note{}, errors.New("can't decrypt: wrong key or password")
		}
	}
	var n Note
	if err := json.Unmarshal(plain, &n); err != nil {
		return Note{}, err
	}
	return n, nil
}

// Token takes a note link in any form (…/n/TOKEN, share.example/TOKEN, with
// or without #fragment) or a bare token, and returns the token.
func Token(ref string) string {
	ref, _, _ = strings.Cut(ref, "#")
	ref, _, _ = strings.Cut(ref, "?")
	if i := strings.Index(ref, "/n/"); i >= 0 {
		ref = ref[i+3:]
	} else if u, err := url.Parse(ref); err == nil && u.Host != "" {
		ref = u.Path
	}
	return strings.Trim(ref, "/")
}
