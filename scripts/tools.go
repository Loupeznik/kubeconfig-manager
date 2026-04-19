//go:build tools

// Package tools pins build-time-only dependencies — modules that the main
// application never imports but that scripts (e.g. scripts/gendocs.go) rely
// on. Without this file, `go mod tidy` trims them because the ignore-tagged
// scripts are invisible to the module graph. The `tools` build tag means this
// file never ships in the binary.
package tools

import (
	_ "github.com/cpuguy83/go-md2man/v2/md2man" // used by cobra/doc man-page renderer via gendocs
	_ "github.com/spf13/cobra/doc"              // gendocs.go
)
