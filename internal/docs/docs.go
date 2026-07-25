// Package docs provides the versioned manual embedded in the henji binary.
package docs

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed docs.md
var body string

//go:embed docs.ja.md
var bodyJapanese string

// Manual returns the plain-Markdown manual for version.
func Manual(version string) string {
	return fmt.Sprintf("# henji %s — manual\n\n%s\n", version, strings.TrimSpace(body))
}

// ManualJapanese returns the Japanese plain-Markdown manual for version.
func ManualJapanese(version string) string {
	return fmt.Sprintf("# henji %s — マニュアル\n\n%s\n", version, strings.TrimSpace(bodyJapanese))
}

// Body returns the unversioned manual body for validation tests.
func Body() string {
	return body
}
