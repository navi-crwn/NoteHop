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

// keyArg is the note key from where the caller put it: -key-file, then -key (which shows
// in the process list and in shell history), then the NOTEHOP_KEY environment variable. Empty when none is given. Each may be a bare key or a whole link.
func keyArg(flagKey, keyFile string) (string, error) {
	if keyFile != "" {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return "", err
		}
		if k := strings.TrimSpace(string(b)); k != "" {
			return k, nil
		}
		return "", fmt.Errorf("%s is empty", keyFile)
	}
	if flagKey != "" {
		return flagKey, nil
	}
	return strings.TrimSpace(os.Getenv("NOTEHOP_KEY")), nil
}

const keyWarning = "the note key: a bare key or the whole link. Visible in the process list and shell history: prefer -key-file or NOTEHOP_KEY"

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
	key := fs.String("key", "", keyWarning+" (for an id without #k=…)")
	keyFile := fs.String("key-file", "", "read the note key (or whole link) from this file; NOTEHOP_KEY works too")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: notehop get [flags] < link.txt          (link, or \"TOKEN KEY\", from stdin)")
		fmt.Fprintln(stderr, "       notehop get [flags] -key-file FILE TOKEN   (or NOTEHOP_KEY=… notehop get TOKEN)")
		fmt.Fprintln(stderr, "       notehop get [flags] 'https://…/TOKEN#k=…'  (the key is then on the command line)")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	k, err := keyArg(*key, *keyFile)
	if err != nil {
		fmt.Fprintln(stderr, "get:", err)
		return 1
	}
	var ref string
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
	// A key given by flag or file always wins; one from NOTEHOP_KEY only fills in a link that has none.
	if explicit := *key != "" || *keyFile != ""; k != "" && (explicit || !strings.Contains(link, "#k=")) {
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
	key := fs.String("key", "", keyWarning)
	keyFile := fs.String("key-file", "", "read the note key (or whole link) from this file; NOTEHOP_KEY works too")
	password := fs.String("password", "", "the note's password, if it has one (or NOTEHOP_PASSWORD)")
	salt := fs.String("salt", "", "the note's salt, from its metadata; needed with a password")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: notehop decrypt -key-file key.txt [file]   (payload from stdin without a file)")
		fmt.Fprintln(stderr, "       NOTEHOP_KEY='https://…/TOKEN#k=…' notehop decrypt [file]")
		fmt.Fprintln(stderr, "Works offline, with the same format the web app writes.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	k, err := keyArg(*key, *keyFile)
	if err != nil {
		fmt.Fprintln(stderr, "decrypt:", err)
		return 1
	}
	if k == "" || fs.NArg() > 1 {
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
	n, err := client.Decrypt(string(raw), k, pw, *salt)
	if err != nil {
		fmt.Fprintln(stderr, "decrypt:", err)
		return 1
	}
	printNote(stdout, n)
	return 0
}
