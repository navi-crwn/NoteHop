package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/navi-crwn/NoteHop/internal/client"
)

// The read side of the command line: "get" (online) and "decrypt" (offline).
// Both take their input from a file or a pipe as well as from arguments, so
// they chain with "notehop new": notehop new note.md | notehop get

// stdinIsTerminal is true when nothing is piped in; the commands then print
// their usage instead of waiting for typing nobody expects to do.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err != nil || fi.Mode()&os.ModeCharDevice != 0
}

func cliGet(args []string) {
	os.Exit(runGet(args, os.Stdin, os.Stdout, os.Stderr, stdinIsTerminal()))
}

func cliDecrypt(args []string) {
	os.Exit(runDecrypt(args, os.Stdin, os.Stdout, os.Stderr, stdinIsTerminal()))
}

func printNote(w io.Writer, n client.Note) {
	if n.Title != "" {
		fmt.Fprintf(w, "# %s\n\n", n.Title)
	}
	fmt.Fprint(w, n.Body)
	if !strings.HasSuffix(n.Body, "\n") {
		fmt.Fprintln(w)
	}
}

// runGet: notehop get [flags] [link]. Without an argument the link (or "ID KEY")
// is the first line of standard input. Returns the exit code.
func runGet(args []string, stdin io.Reader, stdout, stderr io.Writer, terminal bool) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	password := fs.String("password", "", "the note's password (or NOTEHOP_PASSWORD)")
	server := fs.String("server", env("NOTEHOP_SERVER", "http://127.0.0.1:8080"), "NoteHop address for a bare note id (or NOTEHOP_SERVER)")
	key := fs.String("key", "", "the note key, for an id without #k=… (a link or fragment works too)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: notehop get [flags] 'https://…/TOKEN#k=…'")
		fmt.Fprintln(stderr, "       notehop get [flags] -key KEY TOKEN")
		fmt.Fprintln(stderr, "       … | notehop get [flags]      (link, or \"TOKEN KEY\", from stdin)")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var ref, k string = "", *key
	switch fs.NArg() {
	case 0:
		if terminal {
			fs.Usage()
			return 2
		}
		raw, err := io.ReadAll(io.LimitReader(stdin, 8<<10))
		if err != nil {
			fmt.Fprintln(stderr, "get:", err)
			return 1
		}
		f := strings.Fields(string(raw))
		if len(f) == 0 {
			fmt.Fprintln(stderr, "get: nothing on stdin (expected a note link, or \"TOKEN KEY\")")
			return 2
		}
		ref = f[0]
		if len(f) > 1 && k == "" {
			k = f[1]
		}
	case 1:
		ref = fs.Arg(0)
	default:
		fs.Usage()
		return 2
	}
	link := ref
	if !strings.Contains(ref, "://") {
		if strings.Contains(ref, "/") { // note.example.com/TOKEN#k=…
			link = "https://" + ref
		} else { // a bare id: it lives on -server
			link = strings.TrimRight(*server, "/") + "/" + client.Token(ref) + fragmentOf(ref)
		}
	}
	if k != "" {
		base, _, _ := strings.Cut(link, "#")
		link = base + "#k=" + client.KeyFrom(k)
	}
	pw := *password
	if pw == "" {
		pw = os.Getenv("NOTEHOP_PASSWORD")
	}
	n, err := client.Get(link, pw)
	if err != nil {
		fmt.Fprintln(stderr, "get:", err)
		return 1
	}
	printNote(stdout, n)
	return 0
}

func fragmentOf(ref string) string {
	if _, f, ok := strings.Cut(ref, "#"); ok {
		return "#" + f
	}
	return ""
}

// runDecrypt: notehop decrypt -key KEY [-password p -salt s] [file]. Reads the
// stored payload (the sealed text, or the JSON of an open request) from the
// file or standard input and prints the note. Offline: it never connects.
func runDecrypt(args []string, stdin io.Reader, stdout, stderr io.Writer, terminal bool) int {
	fs := flag.NewFlagSet("decrypt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	key := fs.String("key", "", "the note key: the bare key, or the whole link with #k=… (required)")
	password := fs.String("password", "", "the note's password, if it has one (or NOTEHOP_PASSWORD)")
	salt := fs.String("salt", "", "the note's salt, from its metadata; needed with a password")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: notehop decrypt -key 'https://…/TOKEN#k=…' [file]   (payload from stdin without a file)")
		fmt.Fprintln(stderr, "Works offline, with the same format the web app writes.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *key == "" || fs.NArg() > 1 {
		fs.Usage()
		return 2
	}
	var in io.Reader = stdin
	if fs.NArg() == 1 && fs.Arg(0) != "-" {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "decrypt:", err)
			return 1
		}
		defer f.Close()
		in = f
	} else if fs.NArg() == 0 && terminal {
		fs.Usage()
		return 2
	}
	raw, err := io.ReadAll(io.LimitReader(in, 16<<20))
	if err != nil {
		fmt.Fprintln(stderr, "decrypt:", err)
		return 1
	}
	pw := *password
	if pw == "" {
		pw = os.Getenv("NOTEHOP_PASSWORD")
	}
	n, err := client.Decrypt(string(raw), *key, pw, *salt)
	if err != nil {
		fmt.Fprintln(stderr, "decrypt:", err)
		return 1
	}
	printNote(stdout, n)
	return 0
}
