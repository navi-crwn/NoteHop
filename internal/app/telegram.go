package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/i18n"
)

// Telegram bot ⚠️: messages sent to the bot are added to a note's inbox.
// /connect <inbox address> links a chat to a note (the address comes from
// the note's advanced settings); after that every message becomes an entry.
// Not end-to-end: Telegram and this server see the text in transit; it is
// stored sealed to the note's key like any inbox entry.
//
// Setup: TELEGRAM_BOT_TOKEN and NOTEHOP_BASE_URL; the webhook is registered
// with Telegram on start.

var telegramAPI = "https://api.telegram.org"

// telegramSecret is both the webhook path and Telegram's secret header, derived
// from the bot token so it stays the same across restarts.
func (a *App) telegramSecret() string {
	h := sha256.Sum256([]byte("notehop/telegram/" + a.cfg.TelegramToken))
	return hex.EncodeToString(h[:16])
}

// SetupTelegram registers the webhook. Call once at start.
func (a *App) SetupTelegram(ctx context.Context) error {
	if a.cfg.TelegramToken == "" {
		return nil
	}
	if a.cfg.BaseURL == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN needs NOTEHOP_BASE_URL")
	}
	return a.telegramCall(ctx, "setWebhook", map[string]any{
		"url":             a.cfg.BaseURL + "/api/telegram/" + a.telegramSecret(),
		"secret_token":    a.telegramSecret(),
		"allowed_updates": []string{"message"},
	})
}

func (a *App) telegramCall(ctx context.Context, method string, body any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", telegramAPI+"/bot"+a.cfg.TelegramToken+"/"+method, bytes.NewReader(mustJSON(body)))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("telegram %s: %s", method, res.Status)
	}
	return nil
}

func (a *App) apiTelegram(w http.ResponseWriter, r *http.Request) {
	secret := a.telegramSecret()
	if a.cfg.TelegramToken == "" ||
		subtle.ConstantTimeCompare([]byte(r.PathValue("secret")), []byte(secret)) != 1 ||
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(secret)) != 1 {
		apiErr(w, http.StatusNotFound, "err.not_found")
		return
	}
	var u struct {
		Message *struct {
			Text string `json:"text"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			From struct {
				FirstName string `json:"first_name"`
				Username  string `json:"username"`
				Lang      string `json:"language_code"`
			} `json:"from"`
		} `json:"message"`
	}
	if !readJSON(w, r, &u, 256<<10) {
		return
	}
	w.WriteHeader(http.StatusOK) // Telegram retries anything else
	m := u.Message
	if m == nil || strings.TrimSpace(m.Text) == "" {
		return
	}
	lang := strings.ToLower(strings.SplitN(m.From.Lang, "-", 2)[0])
	if !i18n.Supported(lang) {
		lang = i18n.Default
	}
	reply := a.telegramHandle(m.Chat.ID, m.From.FirstName, m.Text, lang)
	go func() {
		if err := a.telegramCall(context.Background(), "sendMessage", map[string]any{"chat_id": m.Chat.ID, "text": reply}); err != nil {
			log.Printf("telegram: %v", err)
		}
	}()
}

func (a *App) telegramHandle(chat int64, name, text, lang string) string {
	text = strings.TrimSpace(text)
	cmd, arg, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@") // /connect@NoteHopBot in groups
	switch cmd {
	case "/start", "/help":
		return i18n.T(lang, "tg.help")
	case "/connect":
		token := strings.TrimSpace(arg)
		if i := strings.LastIndex(token, "/in/"); i >= 0 {
			token = token[i+4:]
		}
		if _, err := a.db.InboxByToken(token); token == "" || err != nil {
			return i18n.T(lang, "tg.bad_inbox")
		}
		if err := a.db.LinkTelegram(chat, token); err != nil {
			return i18n.T(lang, "err.save_failed")
		}
		return i18n.T(lang, "tg.connected")
	case "/disconnect":
		a.db.UnlinkTelegram(chat)
		return i18n.T(lang, "tg.disconnected")
	}
	token, err := a.db.TelegramInbox(chat)
	if err != nil {
		return i18n.T(lang, "tg.not_connected")
	}
	if name == "" {
		name = "Telegram"
	}
	if err := a.deliverInbox(token, clipRunes(name, 80), text); err != nil {
		if err == errInboxGone {
			a.db.UnlinkTelegram(chat)
			return i18n.T(lang, "tg.bad_inbox")
		}
		return i18n.T(lang, "err.full")
	}
	return i18n.T(lang, "tg.saved")
}
