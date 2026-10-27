// Command notehop runs the NoteHop server: one binary, one SQLite file.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/navi-crwn/NoteHop/internal/app"
	"github.com/navi-crwn/NoteHop/internal/db"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		backup(os.Args[2:])
		return
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
		SSOURL:     strings.TrimRight(env("NOTEHOP_SSO_URL", ""), "/"),
		SSOSecret:  env("NOTEHOP_SSO_SECRET", ""),
		ShareURL:      strings.TrimRight(env("NOTEHOP_SHARE_URL", ""), "/"),
	}
	if cfg.ShareURL != "" && base == "" {
		log.Printf("NOTEHOP_SHARE_URL is ignored: it needs NOTEHOP_BASE_URL for the main site")
	}
	if cfg.SSOURL != "" && !cfg.SSOEnabled() {
		log.Printf("SSO sign-in is off: it needs NOTEHOP_SSO_SECRET (32+ chars) and NOTEHOP_BASE_URL")
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
