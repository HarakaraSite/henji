package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestNoTTYStylesContainNoANSI(t *testing.T) {
	s := makeStyles(colorprofile.NoTTY)
	rendered := strings.Join([]string{
		s.AppName.Render("henji"),
		s.ErrorHeader.String(),
		s.Flag.Render("--help"),
		s.InlineCode.Render("henji docs"),
		s.Link.Render("example"),
	}, " ")

	if strings.Contains(rendered, "\x1b[") {
		t.Fatalf("non-TTY styles contain ANSI escapes: %q", rendered)
	}
}
