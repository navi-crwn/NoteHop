// Command notehop runs the NoteHop server: one binary, one SQLite file.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/navi-crwn/NoteHop/internal/app"
	"github.com/navi-crwn/NoteHop/internal/client"
	"github.com/navi-crwn/NoteHop/internal/db"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

// envList reads "a, b,c" as lowercase names.
func envList(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// envLinks reads "Name=https://…" pairs, comma separated, for the footer.
// Only http(s) addresses are kept.
func envLinks(key string) []app.Link {
	var out []app.Link
	for _, v := range strings.Split(os.Getenv(key), ",") {
		name, u, ok := strings.Cut(strings.TrimSpace(v), "=")
		name, u = strings.TrimSpace(name), strings.TrimSpace(u)
		if ok && name != "" && (strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")) {
			out = append(out, app.Link{Name: name, URL: u})
		}
	}
	return out
}

// envOrigins reads "https://ai.example.com,https://proxy.example.org": bare
// origins only (scheme://host[:port]), so they are safe inside a CSP.
func envOrigins(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		u, err := url.Parse(strings.TrimSpace(v))
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.Path == "" && u.RawQuery == "" && u.User == nil &&
			!strings.ContainsAny(u.Host, " ;,'*") {
			out = append(out, u.Scheme+"://"+u.Host)
		}
	}
	return out
}

var providerID = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// envProviders reads "google=Google,github=GitHub": the id the account server
// knows and the name on the button.
func envProviders(key string) []app.Link {
	var out []app.Link
	for _, v := range strings.Split(os.Getenv(key), ",") {
		id, name, ok := strings.Cut(strings.TrimSpace(v), "=")
		id, name = strings.TrimSpace(id), strings.TrimSpace(name)
		if ok && name != "" && providerID.MatchString(id) {
			out = append(out, app.Link{Name: name, URL: id})
		}
	}
	return out
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "backup":
			backup(os.Args[2:])
			return
		case "new":
			cliNew(os.Args[2:])
			return
		case "get":
			cliGet(os.Args[2:])
			return
		case "takedown":
			takedown(os.Args[2:])
			return
		}
	}
	showVersion := flag.Bool("version", false, "print version and exit")
	addr := flag.String("addr", env("NOTEHOP_ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", env("NOTEHOP_DB", "notehop.db"), "SQLite database file")
	flag.Parse()
	if *showVersion {
		fmt.Println("notehop", version)
		return
	}

	base := strings.TrimRight(env("NOTEHOP_BASE_URL", ""), "/")
	cfg := app.Config{
		Site:          env("NOTEHOP_SITE", "NoteHop"),
		BaseURL:       base,
		SecureCookies: envBool("NOTEHOP_SECURE_COOKIES", strings.HasPrefix(base, "https://")),
		TrustProxy:    envBool("NOTEHOP_TRUST_PROXY", false),
		Registration:  envBool("NOTEHOP_REGISTRATION", true),
		SSOURL:        strings.TrimRight(env("NOTEHOP_SSO_URL", ""), "/"),
		SSOSecret:     env("NOTEHOP_SSO_SECRET", ""),
		SSOName:       env("NOTEHOP_SSO_NAME", "SSO"),
		SSOClient:     env("NOTEHOP_SSO_CLIENT", "notehop"),
		SSOProviders:  envProviders("NOTEHOP_SSO_PROVIDERS"),
		Links:         envLinks("NOTEHOP_LINKS"),
		ShareURL:      strings.TrimRight(env("NOTEHOP_SHARE_URL", ""), "/"),
		FileMB:        envInt("NOTEHOP_FILE_MB", 10),
		FilesTotalMB:  envInt("NOTEHOP_FILES_TOTAL_MB", 2048),
		FilesPerIPMB:  envInt("NOTEHOP_FILES_IP_MB", 100),
		AI:            strings.ToLower(env("NOTEHOP_AI", "on")),
		GitHub:        env("NOTEHOP_GITHUB", "navi-crwn/NoteHop"),
		AIOrigins:     envOrigins("NOTEHOP_AI_ORIGINS"),
		PrivateHooks:  envBool("NOTEHOP_PRIVATE_HOOKS", false),
		TelegramToken: env("TELEGRAM_BOT_TOKEN", ""),
		Admins:        envList("NOTEHOP_ADMINS"),
		Reserved:      envList("NOTEHOP_RESERVED"),
		Contact:       env("NOTEHOP_CONTACT", ""),
		HopClients:    envList("NOTEHOP_HOP_CLIENTS"),
		SSHAddr:       env("NOTEHOP_SSH_ADDR", ""),
		Version:       version,
	}
	if cfg.ShareURL != "" && base == "" {
		log.Printf("NOTEHOP_SHARE_URL is ignored: it needs NOTEHOP_BASE_URL for the main site")
	}
	if cfg.SSOURL != "" && !cfg.SSOEnabled() {
		log.Printf("single sign-on is off: it needs NOTEHOP_SSO_SECRET (32+ chars) and NOTEHOP_BASE_URL")
	}

	d, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer d.Close()
	a, err := app.New(cfg, d)
	if err != nil {
		log.Fatalf("app: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go a.Janitor(ctx)
	if err := a.SetupTelegram(ctx); err != nil {
		log.Printf("telegram bot is off: %v", err)
	}
	if sshAddr := cfg.SSHAddr; sshAddr != "" {
		key := env("NOTEHOP_SSH_KEY", filepath.Join(filepath.Dir(*dbPath), "ssh_host_ed25519_key"))
		go func() {
			if err := a.ServeSSH(ctx, sshAddr, key); err != nil {
				log.Printf("ssh: %v", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Printf("NoteHop %s listening on %s (db %s)", version, *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shut)
}

// backup copies the live database: notehop backup [-db file] out.db
// Safe while the server runs, so it needs no sqlite3 tool on the server.
func backup(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	dbPath := fs.String("db", env("NOTEHOP_DB", "notehop.db"), "SQLite database file")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: notehop backup [-db notehop.db] out.db")
		os.Exit(2)
	}
	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatalf("backup: %v", err)
	}
	d, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer d.Close()
	if err := d.Backup(fs.Arg(0)); err != nil {
		log.Fatalf("backup: %v", err)
	}
}

// cliNew: notehop new [flags] [file] — encrypts here, then uploads.
func cliNew(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	server := fs.String("server", env("NOTEHOP_SERVER", "http://127.0.0.1:8080"), "NoteHop address (or NOTEHOP_SERVER)")
	title := fs.String("title", "", "title (default: a leading \"# \" line)")
	mode := fs.String("mode", "read", "link for others: read, edit, append or check")
	password := fs.String("password", "", "extra password (or NOTEHOP_PASSWORD)")
	expires := fs.Duration("expires", 0, "delete after, e.g. 1h or 168h")
	burn := fs.Int("burn", 0, "self-destruct after this many views")
	public := fs.Bool("public", false, "no encryption: readable by the server and /raw")
	slug := fs.String("slug", "", "custom address /n/<slug>")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: notehop new [flags] [file]   (text from stdin without a file)")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	var in io.Reader = os.Stdin
	if fs.NArg() > 0 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		in = f
	}
	raw, err := io.ReadAll(io.LimitReader(in, 1<<20))
	if err != nil {
		log.Fatal(err)
	}
	body := strings.TrimRight(string(raw), "\n") + "\n"
	t := *title
	if t == "" && strings.HasPrefix(body, "# ") {
		first, rest, _ := strings.Cut(body, "\n")
		t, body = strings.TrimSpace(first[2:]), strings.TrimLeft(rest, "\n")
	}
	pw := *password
	if pw == "" {
		pw = os.Getenv("NOTEHOP_PASSWORD")
	}
	c, err := client.Create(*server, client.Note{Title: t, Body: body}, client.Options{
		Mode: *mode, Password: pw, Public: *public, ExpiresIn: int(expires.Seconds()), MaxViews: *burn, Slug: *slug,
	})
	if err != nil {
		log.Fatalf("new: %v", err)
	}
	fmt.Println(c.Share)
	fmt.Fprintln(os.Stderr, "admin (keep private):", c.Admin)
}

// cliGet: notehop get <link> — downloads and decrypts here.
func cliGet(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	password := fs.String("password", "", "the note's password (or NOTEHOP_PASSWORD)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: notehop get [-password p] 'https://…/TOKEN#k=…'")
		os.Exit(2)
	}
	pw := *password
	if pw == "" {
		pw = os.Getenv("NOTEHOP_PASSWORD")
	}
	n, err := client.Get(fs.Arg(0), pw)
	if err != nil {
		log.Fatalf("get: %v", err)
	}
	if n.Title != "" {
		fmt.Printf("# %s\n\n", n.Title)
	}
	fmt.Print(n.Body)
	if !strings.HasSuffix(n.Body, "\n") {
		fmt.Println()
	}
}

// takedown removes a reported note (by its link, with or without the #key)
// or an attachment, straight from the database, for the site operator.
func takedown(args []string) {
	fs := flag.NewFlagSet("takedown", flag.ExitOnError)
	dbPath := fs.String("db", env("NOTEHOP_DB", "notehop.db"), "SQLite database file")
	file := fs.Bool("file", false, "the argument is an attachment id (nh-file:ID) instead of a note link")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: notehop takedown [-db notehop.db] https://…/n/TOKEN   (or a bare TOKEN)")
		fmt.Fprintln(os.Stderr, "       notehop takedown [-db notehop.db] -file FILE_ID")
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	d, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer d.Close()
	ref := fs.Arg(0)
	if *file {
		id := strings.TrimPrefix(strings.SplitN(ref, "#", 2)[0], "nh-file:")
		if ok, err := d.DeleteFile(id); err != nil || !ok {
			log.Fatalf("takedown: attachment %q not found (%v)", id, err)
		}
		fmt.Println("attachment deleted:", id)
		return
	}
	token := client.Token(ref)
	n, _, err := d.NoteByLink(token)
	if err != nil {
		log.Fatalf("takedown: no note for %q", token)
	}
	if err := d.Takedown(n.ID); err != nil {
		log.Fatalf("takedown: %v", err)
	}
	fmt.Printf("note deleted (all its links); its link names are held for a year\n")
}
