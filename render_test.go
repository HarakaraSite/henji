package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type glowInvocation struct {
	Args  []string `json:"args"`
	Text  string   `json:"text"`
	Pager string   `json:"pager"`
	TUI   string   `json:"tui"`
}

func TestGlowHelperProcess(t *testing.T) {
	mode := os.Getenv("HENJI_TEST_GLOW_HELPER")
	if mode == "" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	text, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	record, _ := json.Marshal(glowInvocation{args, string(text), os.Getenv("GLOW_PAGER"), os.Getenv("GLOW_TUI")})
	if os.WriteFile(os.Getenv("HENJI_TEST_GLOW_RECORD"), record, 0o600) != nil {
		os.Exit(3)
	}
	switch mode {
	case "fail":
		fmt.Print("partial rendering that must not be displayed")
		fmt.Fprint(os.Stderr, "renderer failed")
		os.Exit(1)
	case "wait":
		time.Sleep(10 * time.Second)
	default:
		fmt.Print("\x1b[1mformatted\x1b[0m\n\n")
	}
	os.Exit(0)
}

func fakeGlow(t *testing.T, mode string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the external-process fixture uses /bin/sh")
	}
	dir := t.TempDir()
	// The shell only execs the test executable, so cancellation kills the actual
	// renderer rather than leaving a fixture child running behind a shell.
	executable := "'" + strings.ReplaceAll(os.Args[0], "'", "'\\''") + "'"
	script := "#!/bin/sh\nexec " + executable + " -test.run=^TestGlowHelperProcess$ -- \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "glow"), []byte(script), 0o700))
	record := filepath.Join(dir, "invocation.json")
	t.Setenv("PATH", dir)
	t.Setenv("HENJI_TEST_GLOW_HELPER", mode)
	t.Setenv("HENJI_TEST_GLOW_RECORD", record)
	t.Setenv("GLAMOUR_STYLE", "")
	return record
}

func captureTextOutput(t *testing.T, tty bool, run func() error) (string, error) {
	t.Helper()
	oldStdout, oldTTY := os.Stdout, isOutputTTY
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout, isOutputTTY = w, func() bool { return tty }
	defer func() {
		os.Stdout, isOutputTTY = oldStdout, oldTTY
		_ = r.Close()
		_ = w.Close()
	}()
	err = run()
	require.NoError(t, w.Close())
	output, readErr := io.ReadAll(r)
	require.NoError(t, readErr)
	return string(output), err
}

func TestTextOutputUsesGlowOnlyAfterCompletion(t *testing.T) {
	record := fakeGlow(t, "success")
	t.Setenv("GLOW_PAGER", "true")
	t.Setenv("GLOW_TUI", "true")
	t.Setenv("GLAMOUR_STYLE", "light")
	m := &Mods{ctx: context.Background(), Config: &Config{Output: "text", WordWrap: 60}}
	output, err := captureTextOutput(t, true, func() error {
		m.appendToOutput("# Heading\n")
		m.appendToOutput("body\n")
		require.NoFileExists(t, record, "rendering must wait for completion")
		return m.printTextOutput()
	})
	require.NoError(t, err)
	require.Equal(t, "\x1b[1mformatted\x1b[0m\n", output)
	data, err := os.ReadFile(record)
	require.NoError(t, err)
	var call glowInvocation
	require.NoError(t, json.Unmarshal(data, &call))
	require.Equal(t, []string{"-w", "60", "-s", "light", "-"}, call.Args)
	require.Equal(t, "# Heading\nbody\n", call.Text)
	require.Equal(t, "false", call.Pager)
	require.Equal(t, "false", call.TUI)
}

func TestTextOutputFallsBackWithoutPartialRendering(t *testing.T) {
	for _, mode := range []string{"missing", "fail"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "missing" {
				t.Setenv("PATH", t.TempDir())
			} else {
				fakeGlow(t, mode)
			}
			m := &Mods{ctx: context.Background(), Config: &Config{WordWrap: 80}, Output: "# original\n"}
			output, err := captureTextOutput(t, true, m.printTextOutput)
			require.NoError(t, err)
			require.Equal(t, "# original\n\n", output)
		})
	}
}

func TestTextOutputKeepsRawAndPipedStreaming(t *testing.T) {
	for _, test := range []struct {
		name string
		tty  bool
		raw  bool
		want string
	}{
		{"raw terminal", true, true, "first second"},
		{"pipe", false, false, "first second\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := fakeGlow(t, "success")
			m := &Mods{ctx: context.Background(), Config: &Config{Raw: test.raw, WordWrap: 80}}
			oldStdout, oldTTY := os.Stdout, isOutputTTY
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout, isOutputTTY = w, func() bool { return test.tty }
			defer func() {
				os.Stdout, isOutputTTY = oldStdout, oldTTY
				_ = r.Close()
				_ = w.Close()
			}()
			require.NoError(t, r.SetReadDeadline(time.Now().Add(time.Second)))
			m.appendToOutput("first ")
			first := make([]byte, len("first "))
			_, err = io.ReadFull(r, first)
			require.NoError(t, err, "the first chunk must be visible before completion")
			m.appendToOutput("second")
			require.NoError(t, m.printTextOutput())
			require.NoError(t, w.Close())
			rest, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Equal(t, test.want, string(first)+string(rest))
			require.NoFileExists(t, record)
		})
	}
}

func TestGlowCancellationDoesNotDumpOriginalText(t *testing.T) {
	record := fakeGlow(t, "wait")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	go func() {
		defer close(ready)
		for {
			if _, err := os.Stat(record); err == nil {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()
	deadline := time.AfterFunc(2*time.Second, cancel)
	defer deadline.Stop()
	m := &Mods{ctx: ctx, Config: &Config{WordWrap: 80}, Output: "original should not be dumped"}
	output, err := captureTextOutput(t, true, m.printTextOutput)
	<-ready
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, output)
	require.FileExists(t, record, "the renderer must have started before cancellation")
}

func TestStructuredOutputDoesNotUseGlow(t *testing.T) {
	record := fakeGlow(t, "success")
	doc, validator, err := loadJSONSchema(writeDecisionQuestions(t, `{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`))
	require.NoError(t, err)
	m := &Mods{ctx: context.Background(), Config: &Config{Output: "text", jsonSchemaDoc: doc, jsonSchemaValidator: validator}}
	output, err := captureTextOutput(t, true, func() error {
		m.appendToOutput(`{"answer":"complete"}`)
		return m.printTextOutput()
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"answer":"complete"}`, output)
	require.NoFileExists(t, record)

	m = &Mods{Config: &Config{Output: "json"}}
	output, err = captureTextOutput(t, true, func() error {
		m.appendToOutput("buffered for JSON envelope")
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, output)
	require.NoFileExists(t, record)
	require.Equal(t, "buffered for JSON envelope", m.Output)
}

func TestCLIDecisionIgnoresInstalledGlow(t *testing.T) {
	record := fakeGlow(t, "success")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"answers":{"relevant":{"type":"noul","noul":1}}}`)
	}))
	defer server.Close()
	env, cachePath := cliTestEnvironment(t, fmt.Sprintf("output: json\napis:\n  gateway:\n    decision-protocol: openrouter\n    decision-base-url: %s\n    api-key: fake\n", server.URL))
	cmd := cliTestCommand(t, env, "decision", "-a", "gateway", "-m", "test", "--questions", writeDecisionQuestions(t, openRouterDecisionQuestions))
	cmd.Stdin = strings.NewReader("input")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	require.JSONEq(t, `{"answers":{"relevant":{"type":"noul","noul":1}}}`, stdout.String())
	require.Empty(t, stderr.String())
	require.NoDirExists(t, cachePath)
	require.NoFileExists(t, record)
}
