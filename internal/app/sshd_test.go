package app

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSSH(t *testing.T) {
	a, d, _ := newAppDB(t)
	a.cfg.BaseURL = "https://note.test"
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := filepath.Join(t.TempDir(), "host")
	go a.ServeSSH(ctx, addr, key)

	run := func(cmd, stdin string) string {
		t.Helper()
		var c *ssh.Client
		var err error
		for i := 0; i < 50; i++ {
			c, err = ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "x", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second})
			if err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		s, _ := c.NewSession()
		var out bytes.Buffer
		s.Stdout = &out
		s.Stdin = strings.NewReader(stdin)
		s.Run(cmd)
		return out.String()
	}

	out := run("new", "# Belanja\n\n- susu\n")
	if !strings.HasPrefix(out, "https://note.test/n/") || !strings.Contains(out, "admin (keep private): ") {
		t.Fatalf("new: %q", out)
	}
	link := strings.SplitN(out, "\n", 2)[0]
	var content string
	d.QueryRow(`SELECT content FROM notes`).Scan(&content)
	if content != `{"t":"Belanja","b":"- susu\n"}` {
		t.Fatalf("stored: %s", content)
	}
	if got := run("get "+link, ""); got != "# Belanja\n\n- susu\n" {
		t.Fatalf("get: %q", got)
	}
	if got := run("new", ""); !strings.HasPrefix(got, "error:") {
		t.Fatalf("empty: %q", got)
	}
	if got := run("help", ""); !strings.Contains(got, "public notes only") {
		t.Fatalf("help: %q", got)
	}
	// The host key survives a restart.
	s1, _ := hostKey(key)
	s2, _ := hostKey(key)
	if !bytes.Equal(s1.PublicKey().Marshal(), s2.PublicKey().Marshal()) {
		t.Fatal("host key changed")
	}
}
