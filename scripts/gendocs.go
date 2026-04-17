//go:build ignore

// gendocs generates Markdown and man-page references for every command
// in the kubeconfig-manager CLI tree. Invoke via:
//
//	go run scripts/gendocs.go
//
// Output goes to docs/cli/*.md and docs/man/*.1.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra/doc"

	"github.com/loupeznik/kubeconfig-manager/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	root.DisableAutoGenTag = true

	mdDir := filepath.Join("docs", "cli")
	manDir := filepath.Join("docs", "man")

	for _, dir := range []string{mdDir, manDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			exit("create %s: %v", dir, err)
		}
		if err := cleanDir(dir); err != nil {
			exit("clean %s: %v", dir, err)
		}
	}

	if err := doc.GenMarkdownTree(root, mdDir); err != nil {
		exit("markdown: %v", err)
	}

	header := &doc.GenManHeader{
		Title:   "KUBECONFIG-MANAGER",
		Section: "1",
		Source:  "kubeconfig-manager",
		Date:    &time.Time{},
	}
	if err := doc.GenManTree(root, header, manDir); err != nil {
		exit("man: %v", err)
	}

	fmt.Printf("generated markdown → %s\n", mdDir)
	fmt.Printf("generated man pages → %s\n", manDir)
}

func cleanDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gendocs: "+format+"\n", args...)
	os.Exit(1)
}
