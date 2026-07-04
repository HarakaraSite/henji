// Package docs provides the versioned manual embedded in the henji binary.
package docs

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed docs.md
var body string

// Manual returns the plain-Markdown manual for version.
func Manual(version string) string {
	return fmt.Sprintf("# henji %s — manual\n\n%s\n", version, strings.TrimSpace(body))
}

// Body returns the unversioned manual body for validation tests.
func Body() string {
	return body
}
