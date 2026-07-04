package main

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/colorprofile"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

var isInputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
})

var isOutputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})

var stdoutStyles = sync.OnceValue(func() styles {
	return makeStyles(colorprofile.Detect(os.Stdout, os.Environ()), darkBackgroundFromEnv())
})

var stderrStyles = sync.OnceValue(func() styles {
	return makeStyles(colorprofile.Detect(os.Stderr, os.Environ()), darkBackgroundFromEnv())
})

// darkBackgroundFromEnv provides a non-blocking initial value. Interactive
// Bubble Tea programs replace it when the terminal answers the background
// color query; standalone commands must not wait for an OSC response.
func darkBackgroundFromEnv() bool {
	parts := strings.FieldsFunc(os.Getenv("COLORFGBG"), func(r rune) bool {
		return r == ';' || r == ':'
	})
	if len(parts) == 0 {
		return true
	}
	background, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || background < 0 || background > 255 {
		return true
	}
	_, _, lightness := termenv.ConvertToRGB(termenv.ANSI256Color(background)).Hsl()
	return lightness < 0.5
}
