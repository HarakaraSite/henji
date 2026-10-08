package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/stretchr/testify/require"
)

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("HENJI_TEST_CLI_PROCESS") != "1" {
		return
	}
	var args []string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("HENJI_TEST_CLI_ARGS")), &args))
	os.Args = append([]string{"henji"}, args...)
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
