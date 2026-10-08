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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"forge.harakara.site/littleisland/henji/v2/internal/cache"
	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("HENJI_TEST_CLI_PROCESS") != "1" {
		return
	}
	var args []string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("HENJI_TEST_CLI_ARGS")), &args))
	os.Args = append([]string{"henji"}, args...)
	if os.Getenv("HENJI_TEST_CLI_QUERY_ONLY") == "1" {
		// Make real SQLite writes fail after normal CLI/database initialization.
		rootCmd.PreRunE = func(_ *cobra.Command, _ []string) error {
			db.db.SetMaxOpenConns(1)
			_, err := db.db.Exec("PRAGMA query_only=ON")
			return err
		}
	}
	main()
	os.Exit(0)
}

func cliTestCommand(t *testing.T, env []string, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLIHelperProcess$")
	cmd.Env = append(append([]string{}, env...), "HENJI_TEST_CLI_PROCESS=1", "HENJI_TEST_CLI_ARGS="+string(encoded))
	cmd.Stdin = strings.NewReader("")
	return cmd
}

func TestConcurrentContinuationsReadUpdatedHistory(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	secondHistory := make(chan []proto.Message, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []proto.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			http.Error(w, "invalid test request", http.StatusBadRequest)
			return
		}
		prompt := request.Messages[len(request.Messages)-1].Content
		switch prompt {
		case "first":
			close(firstStarted)
			select {
			case <-releaseFirst:
			case <-r.Context().Done():
				return
			}
		case "second":
			secondHistory <- request.Messages
		}
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := map[string]any{
			"id": "chat-test", "object": "chat.completion.chunk", "created": 1, "model": "test",
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "answer to " + prompt}}},
		}
		encoded, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
	}))
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseFirst) })
		server.Close()
	})

	dir := t.TempDir()
	configHome, dataHome := filepath.Join(dir, "config"), filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(filepath.Join(configHome, "henji"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configHome, "henji", "henji.yml"), []byte(fmt.Sprintf(
		"default-api: local\ndefault-model: test\napis:\n  local:\n    base-url: %s/v1\n    api-key: fake\n    models:\n      test: {}\n", server.URL)), 0o600))
	conversations, err := cache.NewConversations(filepath.Join(dataHome, "henji"))
	require.NoError(t, err)
	id := newConversationID()
	initial := []proto.Message{{Role: proto.RoleUser, Content: "original"}}
	require.NoError(t, conversations.Write(id, &initial))
	index, err := openDB(filepath.Join(dataHome, "henji", "conversations", "henji.db"))
	require.NoError(t, err)
	require.NoError(t, index.Save(id, "original", "local", "test"))
	require.NoError(t, index.Close())

	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "HENJI_") && !strings.HasPrefix(value, "XDG_CONFIG_HOME=") && !strings.HasPrefix(value, "XDG_DATA_HOME=") {
			env = append(env, value)
		}
	}
	env = append(env, "XDG_CONFIG_HOME="+configHome, "XDG_DATA_HOME="+dataHome)
	start := func(prompt string, continuing bool) (*exec.Cmd, *bytes.Buffer) {
		t.Helper()
		args := []string{"--quiet", "--output", "json"}
		if continuing {
			args = append(args, "--continue", id)
		}
		cmd := cliTestCommand(t, env, append(args, prompt)...)
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		require.NoError(t, cmd.Start())
		return cmd, &output
	}
	first, firstOutput := start("first", true)
	select {
	case <-firstStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first continuation did not reach the API")
	}
	second, secondOutput := start("second", true)
	// Another conversation can finish while the first request holds its lock.
	independent, independentOutput := start("independent", false)
	require.NoError(t, independent.Wait(), "%s", independentOutput)
	releaseOnce.Do(func() { close(releaseFirst) })
	require.NoError(t, first.Wait(), "%s", firstOutput)
	require.NoError(t, second.Wait(), "%s", secondOutput)

	select {
	case history := <-secondHistory:
		require.Contains(t, history, proto.Message{Role: proto.RoleAssistant, Content: "answer to first"})
	case <-time.After(time.Second):
		t.Fatal("second continuation did not reach the API")
	}
	var saved []proto.Message
	require.NoError(t, conversations.Read(id, &saved))
	require.Equal(t, []proto.Message{
		{Role: proto.RoleUser, Content: "original"},
		{Role: proto.RoleUser, Content: "first"},
		{Role: proto.RoleAssistant, Content: "answer to first"},
		{Role: proto.RoleUser, Content: "second"},
		{Role: proto.RoleAssistant, Content: "answer to second"},
	}, saved)
}

func cliTestEnvironment(t *testing.T, configText string) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	configHome, dataHome := filepath.Join(dir, "config"), filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(filepath.Join(configHome, "henji"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(configHome, "henji", "henji.yml"), []byte(configText), 0o600))
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "HENJI_") && !strings.HasPrefix(value, "XDG_CONFIG_HOME=") && !strings.HasPrefix(value, "XDG_DATA_HOME=") {
			env = append(env, value)
		}
	}
	env = append(env, "XDG_CONFIG_HOME="+configHome, "XDG_DATA_HOME="+dataHome)
	return env, filepath.Join(dataHome, "henji")
}

func TestCLIJSONResponseFailures(t *testing.T) {
	const answer = "日本語の応答\nsecond line"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []proto.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(request.Messages) > 0 && request.Messages[len(request.Messages)-1].Content == "unauthorized" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"message":"test authentication failure","type":"invalid_request_error","code":"invalid_api_key"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := map[string]any{
			"id": "chat-test", "object": "chat.completion.chunk", "created": 1, "model": "test",
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": answer}}},
		}
		encoded, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
	}))
	t.Cleanup(server.Close)
	cases := []struct {
		name, errorText string
		args            []string
		failSave        bool
		badDB           bool
		withAnswer      bool
	}{
		{name: "success", args: []string{"prompt"}, withAnswer: true},
		{name: "no cache", args: []string{"--no-cache", "prompt"}, withAnswer: true},
		{name: "missing schema", args: []string{"--json-schema", "missing-schema.json", "prompt"}, errorText: "could not read --json-schema"},
		{name: "missing text file", args: []string{"--text", "missing-input.txt", "prompt"}, errorText: "Could not read text input"},
		{name: "missing prompt", errorText: "You haven't provided any prompt input"},
		{name: "unknown model", args: []string{"--model", "not-configured", "prompt"}, errorText: "not-configured"},
		{name: "invalid flag", args: []string{"--invalid-json-test-flag"}, errorText: "unknown flag"},
		{name: "database initialization failure", badDB: true, args: []string{"prompt"}, errorText: "Could not open database"},
		{name: "provider failure", args: []string{"unauthorized"}, errorText: "Invalid local API key"},
		{name: "failed continuation save", args: []string{"follow up"}, failSave: true, withAnswer: true, errorText: "problem writing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, dataDir := cliTestEnvironment(t, fmt.Sprintf("default-api: local\ndefault-model: test\napis:\n  local:\n    base-url: %s/v1\n    api-key: fake\n    models:\n      test: {}\n", server.URL))
			args := []string{"--quiet", "--output", "json"}
			if tc.badDB {
				require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "conversations", "henji.db"), 0o700))
			}
			var bodyPath, id string
			var oldBody []byte
			if tc.failSave {
				conversations, err := cache.NewConversations(dataDir)
				require.NoError(t, err)
				id = newConversationID()
				messages := []proto.Message{{Role: proto.RoleUser, Content: "old question"}}
				require.NoError(t, conversations.Write(id, &messages))
				bodyPath = filepath.Join(dataDir, "conversations", id+".gob")
				oldBody, err = os.ReadFile(bodyPath)
				require.NoError(t, err)
				index, err := openDB(filepath.Join(dataDir, "conversations", "henji.db"))
				require.NoError(t, err)
				require.NoError(t, index.Save(id, "old title", "local", "test"))
				require.NoError(t, index.Close())
				args = append(args, "--continue", id)
				env = append(env, "HENJI_TEST_CLI_QUERY_ONLY=1")
			}
			cmd := cliTestCommand(t, env, append(args, tc.args...)...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if tc.errorText == "" {
				require.NoError(t, err, "%s", &stderr)
			} else {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr, "%s", &stderr)
				require.Equal(t, 1, exitErr.ExitCode())
			}
			require.Equal(t, 1, strings.Count(stdout.String(), "\n"), "stdout must be a single JSON line")
			var out JSONOutput
			decoder := json.NewDecoder(&stdout)
			require.NoError(t, decoder.Decode(&out), "%s", &stderr)
			require.Equal(t, jsonSchemaVersion, out.Version)
			require.Equal(t, io.EOF, decoder.Decode(new(any)), "stdout must contain only one JSON envelope")
			if tc.errorText == "" {
				require.Nil(t, out.Error)
			} else {
				require.NotNil(t, out.Error)
				require.Equal(t, "error", out.Error.Code)
				require.Contains(t, out.Error.Message, tc.errorText)
			}
			if tc.withAnswer {
				require.Equal(t, []ContentBlock{{Type: "text", Text: answer}}, out.Content)
			} else {
				require.Empty(t, out.Content)
			}
			if tc.failSave {
				require.Equal(t, id, out.ConversationID)
				body, err := os.ReadFile(bodyPath)
				require.NoError(t, err)
				require.Equal(t, oldBody, body)
			}
		})
	}
}
