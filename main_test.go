package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	manualdocs "forge.harakara.site/littleisland/henji/internal/docs"
)

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
	initFlags()
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
