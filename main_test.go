package main

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	manualdocs "forge.harakara.site/littleisland/henji/v2/internal/docs"
)

func TestConversationUpdatedAtUsesLocalTime(t *testing.T) {
	original := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	t.Cleanup(func() { time.Local = original })

	updatedAt := time.Date(2026, time.July, 11, 12, 34, 56, 0, time.UTC)
	if got, want := conversationUpdatedAt(updatedAt), "2026-07-11 21:34:56 JST"; got != want {
		t.Fatalf("conversationUpdatedAt() = %q, want %q", got, want)
	}
}

// initFlags registers its flags on the shared rootCmd, so tests must only
// call it once per process or pflag panics on redefinition.
var initFlagsOnce sync.Once

func ensureFlagsInitialized() {
	initFlagsOnce.Do(initFlags)
}

func TestIsDocsCmd(t *testing.T) {
	for args, want := range map[string]bool{
		"":            false,
		"docs":        true,
		"docs -h":     true,
		"docs --help": true,
		"docs extra":  true,
		"say docs":    false,
	} {
		t.Run(args, func(t *testing.T) {
			vargs := append([]string{"henji"}, strings.Fields(args)...)
			if got := isDocsCmd(vargs); got != want {
				t.Fatalf("isDocsCmd(%v) = %v, want %v", vargs, got, want)
			}
		})
	}
}

func TestDocsCommandPrintsPlainMarkdown(t *testing.T) {
	cmd := newDocsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	requireNoError(t, cmd.Execute())

	got := out.String()
	if !strings.HasPrefix(got, "# henji ") {
		t.Fatalf("docs output has unexpected heading: %q", got[:min(len(got), 80)])
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatal("docs output contains ANSI escapes")
	}
}

// TestDocsCommandRejectsArgsWithHelpfulError is a C-2 regression test: a
// prompt starting with the word "docs" (e.g. "docs って何") is parsed as
// this subcommand with trailing arguments. The error must tell the user to
// quote the prompt instead of cobra's generic "unknown command" message.
func TestDocsCommandRejectsArgsWithHelpfulError(t *testing.T) {
	cmd := newDocsCmd()
	cmd.SetArgs([]string{"って何"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for docs with trailing arguments")
	}
	if !strings.Contains(err.Error(), "quote it") {
		t.Fatalf("error does not mention quoting the prompt: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "って何") {
		t.Fatalf("error does not echo the offending arguments: %q", err.Error())
	}
}

func TestRootCommandAcceptsPromptArgumentsAlongsideSubcommands(t *testing.T) {
	cmd, args, err := rootCmd.Find([]string{"explain", "this error"})
	requireNoError(t, err)
	if cmd != rootCmd {
		t.Fatalf("prompt resolved to %q instead of root command", cmd.Name())
	}
	if strings.Join(args, " ") != "explain this error" {
		t.Fatalf("prompt arguments changed: %q", args)
	}
}

func TestManualLongFlagsExist(t *testing.T) {
	ensureFlagsInitialized()
	re := regexp.MustCompile(`--([a-z][a-z0-9-]*)`)
	seen := map[string]bool{}
	for _, match := range re.FindAllStringSubmatch(manualdocs.Body(), -1) {
		name := match[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if rootCmd.Flags().Lookup(name) == nil {
			t.Errorf("manual documents unknown flag --%s", name)
		}
	}
}

func TestInteractiveFlagsAreRemoved(t *testing.T) {
	ensureFlagsInitialized()
	for _, name := range []string{"editor", "settings"} {
		if rootCmd.Flags().Lookup(name) != nil {
			t.Errorf("--%s must not be registered", name)
		}
	}
}

func TestFileUsesShortFAndFormatIsLongOnly(t *testing.T) {
	ensureFlagsInitialized()
	file := rootCmd.Flags().Lookup("file")
	if file == nil {
		t.Fatal("--file is not registered")
	}
	if file.Shorthand != "f" {
		t.Fatalf("--file shorthand = %q, want f", file.Shorthand)
	}
	format := rootCmd.Flags().Lookup("format")
	if format == nil {
		t.Fatal("--format is not registered")
	}
	if format.Shorthand != "" {
		t.Fatalf("--format shorthand = %q, want empty", format.Shorthand)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestIsCompletionCmd(t *testing.T) {
	for args, is := range map[string]bool{
		"":                                     false,
		"something":                            false,
		"something something":                  false,
		"completion for my bash script how to": false,
		"completion bash how to":               false,
		"completion":                           false,
		"completion -h":                        true,
		"completion --help":                    true,
		"completion help":                      true,
		"completion bash":                      true,
		"completion fish":                      true,
		"completion zsh":                       true,
		"completion powershell":                true,
		"completion bash -h":                   true,
		"completion fish -h":                   true,
		"completion zsh -h":                    true,
		"completion powershell -h":             true,
		"completion bash --help":               true,
		"completion fish --help":               true,
		"completion zsh --help":                true,
		"completion powershell --help":         true,
		"__complete":                           true,
		"__complete blah blah blah":            true,
	} {
		t.Run(args, func(t *testing.T) {
			vargs := append([]string{"mods"}, strings.Fields(args)...)
			if b := isCompletionCmd(vargs); b != is {
				t.Errorf("%v: expected %v, got %v", vargs, is, b)
			}
		})
	}
}

func TestExecuteFlagErrorDoesNotPanicWithNilDB(t *testing.T) {
	origDB := db
	db = nil
	t.Cleanup(func() { db = origDB })

	ensureFlagsInitialized()
	rootCmd.SetArgs([]string{"--bogus", "-h"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected a flag parse error for --bogus")
	}

	// Regression for F-1: this used to panic with a nil pointer dereference
	// because db stays nil for -h/-v invocations, and main() unconditionally
	// called db.Close() on the Execute() error path.
	closeDB()
}

func TestIsManCmd(t *testing.T) {
	for args, is := range map[string]bool{
		"":                    false,
		"something":           false,
		"something something": false,
		"man is no more":      false,
		"mans":                false,
		"man foo":             false,
		"man":                 true,
		"man -h":              true,
		"man --help":          true,
	} {
		t.Run(args, func(t *testing.T) {
			vargs := append([]string{"mods"}, strings.Fields(args)...)
			if b := isManCmd(vargs); b != is {
				t.Errorf("%v: expected %v, got %v", vargs, is, b)
			}
		})
	}
}
