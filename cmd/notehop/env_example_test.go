package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every variable the program reads is named in the example env file, set or as a comment, so nobody has to read the code to
// find a setting.
func TestEnvExampleNamesEveryVariable(t *testing.T) {
	read := regexp.MustCompile(`(?:Getenv|LookupEnv|env[A-Za-z]*)\("([A-Z][A-Z0-9_]+)"`)
	example, err := os.ReadFile("../../deploy/notehop.env.example")
	if err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^[# ]*([A-Z][A-Z0-9_]+)=`).FindAllStringSubmatch(string(example), -1) {
		named[m[1]] = true
	}
	for _, root := range []string{"../../internal", "."} {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range read.FindAllStringSubmatch(string(b), -1) {
				if !named[m[1]] {
					t.Errorf("%s reads %s, which the example env file does not name", path, m[1])
				}
			}
			return nil
		})
	}
}
