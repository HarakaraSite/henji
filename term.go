package main

import (
	"os"
	"sync"

	"github.com/charmbracelet/colorprofile"
	"github.com/mattn/go-isatty"
)

var isInputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
})

var isOutputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})

var stdoutStyles = sync.OnceValue(func() styles {
	return makeStyles(colorprofile.Detect(os.Stdout, os.Environ()))
})

var stderrStyles = sync.OnceValue(func() styles {
	return makeStyles(colorprofile.Detect(os.Stderr, os.Environ()))
})
