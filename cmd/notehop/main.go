// Command notehop runs the NoteHop server: one binary, one SQLite file.
package main

import (
	"context"
	"errors"
	"flag"
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

func main() {
	addr := flag.String("addr", env("NOTEHOP_ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", env("NOTEHOP_DB", "notehop.db"), "SQLite database file")
	flag.Parse()

	base := strings.TrimRight(env("NOTEHOP_BASE_URL", ""), "/")
	cfg := app.Config{
		Site:          env("NOTEHOP_SITE", "NoteHop"),
		BaseURL:       base,
		SecureCookies: envBool("NOTEHOP_SECURE_COOKIES", strings.HasPrefix(base, "https://")),
		TrustProxy:    envBool("NOTEHOP_TRUST_PROXY", false),
		Registration:  envBool("NOTEHOP_REGISTRATION", true),
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
		log.Printf("NoteHop listening on %s (db %s)", *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shut)
}
