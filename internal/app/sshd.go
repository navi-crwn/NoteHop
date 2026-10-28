package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	nhclient "github.com/navi-crwn/NoteHop/internal/client"
	"github.com/navi-crwn/NoteHop/internal/db"
)

// SSH access for public notes, for terminals without the CLI:
//
//	ssh -p 2222 note.example.com < todo.md       new public note, prints its links
//	ssh -p 2222 note.example.com new "Title" < f same, with a title
//	ssh -p 2222 note.example.com get <link>      prints a public note
//
// ⚠️ Not end-to-end: SSH encrypts the connection, but the server reads the
// text to store it. Notes made here are public (like /raw) and expire after
// 30 days. Anyone may connect; there are no accounts over SSH.

const sshExpiry = 30 * 24 * time.Hour

// ServeSSH listens on addr until ctx ends. The host key is created at
// keyPath on first start and reused after, so clients don't see a new key.
func (a *App) ServeSSH(ctx context.Context, addr, keyPath string) error {
	signer, err := hostKey(keyPath)
	if err != nil {
		return err
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true, ServerVersion: "SSH-2.0-NoteHop"}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); ln.Close() }()
	log.Printf("SSH for public notes on %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		go a.sshConn(c, cfg)
	}
}

func hostKey(path string) (ssh.Signer, error) {
	if b, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(b)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(priv, "notehop host key")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(priv)
}

func (a *App) sshConn(nc net.Conn, cfg *ssh.ServerConfig) {
	defer nc.Close()
	nc.SetDeadline(time.Now().Add(2 * time.Minute)) // whole session, uploads included
	ip, _, _ := net.SplitHostPort(nc.RemoteAddr().String())
	if !a.limit.allow("s"+ip, 20) {
		return
	}
	conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			nch.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, in, err := nch.Accept()
		if err != nil {
			return
		}
		a.sshSession(ch, in)
		return // one command per connection
	}
}

func (a *App) sshSession(ch ssh.Channel, in <-chan *ssh.Request) {
	defer ch.Close()
	cmd := ""
wait: // until the client says what to run
	for req := range in {
		switch req.Type {
		case "exec":
			var p struct{ Cmd string }
			ssh.Unmarshal(req.Payload, &p)
			cmd = p.Cmd
			req.Reply(true, nil)
			break wait
		case "shell":
			req.Reply(true, nil)
			break wait
		case "pty-req", "env":
			req.Reply(true, nil)
		default:
			req.Reply(false, nil)
		}
	}
	go func() {
		for req := range in {
			req.Reply(false, nil)
		}
	}()
	out := a.sshRun(cmd, ch)
	io.WriteString(ch, out)
	status := make([]byte, 4)
	if strings.HasPrefix(out, "error:") {
		binary.BigEndian.PutUint32(status, 1)
	}
	ch.SendRequest("exit-status", false, status)
}

const sshHelp = `NoteHop over SSH (public notes only, not end-to-end encrypted):
  ssh HOST < file.md            new public note from the file
  ssh HOST new "Title" < file   same, with a title
  ssh HOST get LINK-OR-SLUG     print a public note
For encrypted notes use the browser or the notehop CLI ("notehop new").
`

func (a *App) sshRun(cmd string, stdin io.Reader) string {
	args := strings.Fields(cmd)
	verb := "new"
	if len(args) > 0 {
		verb, args = args[0], args[1:]
	}
	switch verb {
	case "help", "-h", "--help":
		return sshHelp
	case "get":
		if len(args) != 1 {
			return "error: usage: get LINK-OR-SLUG\n"
		}
		return a.sshGet(args[0])
	case "new":
		title := strings.Trim(strings.Join(args, " "), `"'`)
		raw, err := io.ReadAll(io.LimitReader(stdin, maxContent))
		if err != nil {
			return "error: " + err.Error() + "\n"
		}
		body := strings.TrimRight(string(raw), "\n")
		if strings.TrimSpace(body) == "" {
			return "error: nothing to save; send the text on stdin\n\n" + sshHelp
		}
		if title == "" && strings.HasPrefix(body, "# ") {
			first, rest, _ := strings.Cut(body, "\n")
			title, body = strings.TrimSpace(first[2:]), strings.TrimLeft(rest, "\n")
		}
		share, admin, err := a.createPublic(clipRunes(title, 200), body+"\n")
		if err != nil {
			return "error: " + err.Error() + "\n"
		}
		return fmt.Sprintf("%s\nadmin (keep private): %s\nexpires in 30 days; public: the server can read it.\n", share, admin)
	}
	return "error: unknown command " + verb + "\n\n" + sshHelp
}

func (a *App) createPublic(title, body string) (share, admin string, err error) {
	k, adm := nhclient.NewKey(), nhclient.NewKey()
	keys, err := nhclient.Derive(k, "", "")
	if err != nil {
		return "", "", err
	}
	content, _ := json.Marshal(nhclient.Note{Title: title, Body: body})
	if len(content) > maxContent {
		return "", "", errors.New("too large")
	}
	n := &db.Note{Content: string(content), AdminHash: hashHex(adm), WriteHash: hashHex(keys.Write),
		Expires: time.Now().Add(sshExpiry).Unix(), Config: "{}"}
	link := &db.Link{Token: randToken(10), Mode: modeRead}
	if err := a.db.CreateNote(n, link); err != nil {
		return "", "", err
	}
	return a.readerLink(link.Token) + "#k=" + k, a.ownerLink(link.Token) + "#k=" + k + "&a=" + adm, nil
}

func (a *App) sshGet(ref string) string {
	n, _, err := a.db.NoteByLink(nhclient.Token(ref))
	if err != nil {
		return "error: not found\n"
	}
	if n.Encrypted {
		return "error: this note is end-to-end encrypted; open it in a browser or with \"notehop get\"\n"
	}
	if n.MaxViews > 0 {
		return "error: self-destructing notes can't be read over SSH\n"
	}
	if e := runBeforeRead(n, time.Now(), ""); e != nil {
		return "error: " + e.key + "\n"
	}
	var c nhclient.Note
	json.Unmarshal([]byte(n.Content), &c)
	if c.Title != "" {
		return "# " + c.Title + "\n\n" + c.Body
	}
	return c.Body
}
