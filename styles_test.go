package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestNoTTYStylesContainNoANSI(t *testing.T) {
	s := makeStyles(colorprofile.NoTTY, true)
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

func TestAdaptiveStylesUseBackgroundVariant(t *testing.T) {
	light := makeStyles(colorprofile.TrueColor, false).Flag.Render("--help")
	dark := makeStyles(colorprofile.TrueColor, true).Flag.Render("--help")
	if light == dark {
		t.Fatalf("light and dark styles are identical: %q", light)
	}
}

func TestDarkBackgroundFromEnv(t *testing.T) {
	for value, want := range map[string]bool{
		"":      true,
		"0;15":  false,
		"15;0":  true,
		"0;255": false,
		"bad":   true,
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("COLORFGBG", value)
			if got := darkBackgroundFromEnv(); got != want {
				t.Fatalf("darkBackgroundFromEnv() = %v, want %v", got, want)
			}
		})
	}
}
