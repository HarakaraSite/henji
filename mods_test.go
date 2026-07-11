package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/cache"
	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"forge.harakara.site/littleisland/henji/v2/internal/stream"
	"github.com/openai/openai-go"
	"github.com/stretchr/testify/require"
)

type fakeMessagesStream struct {
	chunks   []string
	messages []proto.Message
	err      error
	index    int
}

func (f *fakeMessagesStream) Next() bool { return f.index < len(f.chunks) }
func (f *fakeMessagesStream) Current() (proto.Chunk, error) {
	chunk := proto.Chunk{Content: f.chunks[f.index]}
	f.index++
	return chunk, nil
}
func (f *fakeMessagesStream) Close() error              { return nil }
func (f *fakeMessagesStream) Err() error                { return f.err }
func (f *fakeMessagesStream) Messages() []proto.Message { return f.messages }

var _ stream.Stream = (*fakeMessagesStream)(nil)

func TestPtrOrNilPreservesExplicitZero(t *testing.T) {
	require.Nil(t, ptrOrNil(float64(-1)))
	require.Nil(t, ptrOrNil(int64(-1)))
	temp := ptrOrNil(float64(0))
	require.NotNil(t, temp)
	require.Zero(t, *temp)
}

func TestShouldShowSpinner(t *testing.T) {
	require.True(t, shouldShowSpinner(true, false))
	require.False(t, shouldShowSpinner(false, false))
	require.False(t, shouldShowSpinner(true, true))
}

func TestReadAllContextStopsWhenCanceled(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := readAllContext(ctx, reader)
		result <- err
	}()
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
}

func TestReadFileInput(t *testing.T) {
	t.Run("reads valid UTF-8 text", func(t *testing.T) {
		path := t.TempDir() + "/input.txt"
		require.NoError(t, os.WriteFile(path, []byte("attached text"), 0o600))
		content, err := readFileInput(path)
		require.NoError(t, err)
		require.Equal(t, "attached text", content)
	})

	t.Run("rejects binary input", func(t *testing.T) {
		path := t.TempDir() + "/input.bin"
		require.NoError(t, os.WriteFile(path, []byte{'a', 0, 'b'}, 0o600))
		_, err := readFileInput(path)
		require.Error(t, err)
		var merr modsError
		require.ErrorAs(t, err, &merr)
		require.Contains(t, merr.Reason(), "binary")
	})
}

func TestJoinInputParts(t *testing.T) {
	require.Equal(t, "file content\n\n\tstdin", joinInputParts("file content", "\tstdin"))
	require.Equal(t, "file content", joinInputParts("file content", " \n"))
}

func TestConsumeCompletionValidatesSchemaAndRetries(t *testing.T) {
	_, schema, err := loadJSONSchema(writeTempSchema(t, `{"type":"object","required":["ok"]}`))
	require.NoError(t, err)

	first := &fakeMessagesStream{
		chunks:   []string{"not json"},
		messages: []proto.Message{{Role: proto.RoleAssistant, Content: "not json"}},
	}
	second := &fakeMessagesStream{
		chunks:   []string{`{"ok":true}`},
		messages: []proto.Message{{Role: proto.RoleAssistant, Content: `{"ok":true}`}},
	}
	m := &Mods{Config: &Config{Output: "json", jsonSchemaValidator: schema, JSONSchemaRetries: 1}}
	var correction string
	err = m.consumeCompletion(first, func(got string) stream.Stream {
		correction = got
		return second
	})
	require.NoError(t, err)
	require.Contains(t, correction, "did not conform")
	require.Equal(t, `{"ok":true}`, m.Output)
	require.Equal(t, 1, m.schemaRetries)
}

func TestConsumeCompletionStopsAfterSchemaRetries(t *testing.T) {
	_, schema, err := loadJSONSchema(writeTempSchema(t, `{"type":"object","required":["ok"]}`))
	require.NoError(t, err)
	m := &Mods{Config: &Config{Output: "json", jsonSchemaValidator: schema, JSONSchemaRetries: 0}}
	err = m.consumeCompletion(&fakeMessagesStream{messages: []proto.Message{{Role: proto.RoleAssistant, Content: "not json"}}}, nil)
	require.Error(t, err)
	var merr modsError
	require.ErrorAs(t, err, &merr)
	require.Contains(t, merr.Reason(), "did not match")
}

func TestRequestErrorFallbackAndContextRetry(t *testing.T) {
	t.Run("404 switches to and retries with the configured fallback", func(t *testing.T) {
		m := &Mods{Config: &Config{API: "openai", Model: "primary", MaxRetries: 2}}
		err := m.handleAPIError(&openai.Error{StatusCode: http.StatusNotFound}, Model{API: "openai", Fallback: "fallback"}, "prompt")
		require.Equal(t, "fallback", m.Config.Model)
		var retry retryRequest
		require.ErrorAs(t, err, &retry)
		require.Equal(t, "prompt", retry.content)
	})

	t.Run("context length error retries with a shorter prompt", func(t *testing.T) {
		m := &Mods{Config: &Config{MaxRetries: 2}}
		prompt := "abcdefghijklmnopqrstuvwxyz"
		err := m.handleAPIError(&openai.Error{
			StatusCode: http.StatusBadRequest,
			Code:       "context_length_exceeded",
			Message:    "This model's maximum context length is 10 tokens. However, your messages resulted in 10 tokens",
		}, Model{API: "openai"}, prompt)
		var retry retryRequest
		require.ErrorAs(t, err, &retry)
		require.Less(t, len(retry.content), len(prompt))
	})

	t.Run("no-limit keeps context errors terminal", func(t *testing.T) {
		m := &Mods{Config: &Config{NoLimit: true, MaxRetries: 2}}
		err := m.handleAPIError(&openai.Error{StatusCode: http.StatusBadRequest, Code: "context_length_exceeded"}, Model{API: "openai"}, "prompt")
		var retry retryRequest
		require.False(t, errors.As(err, &retry))
	})
}

func TestCompleteRetriesFallbackModel(t *testing.T) {
	m := &Mods{Config: &Config{API: "openai", Model: "primary", Output: "json", MaxRetries: 2}}
	calls := 0
	err := m.completeWith("prompt", func(content string) (stream.Stream, func(string) stream.Stream, Model, error) {
		calls++
		if calls == 1 {
			require.Equal(t, "primary", m.Config.Model)
			return &fakeMessagesStream{err: &openai.Error{StatusCode: http.StatusNotFound}}, nil, Model{API: "openai", Fallback: "fallback"}, nil
		}
		require.Equal(t, "fallback", m.Config.Model)
		return &fakeMessagesStream{
			chunks:   []string{"recovered"},
			messages: []proto.Message{{Role: proto.RoleAssistant, Content: "recovered"}},
		}, nil, Model{API: "openai"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, "recovered", m.Output)
}

func TestReadFromCacheBuildsShowOutputWithoutStreaming(t *testing.T) {
	conversations, err := cache.NewConversations(t.TempDir())
	require.NoError(t, err)
	messages := []proto.Message{{Role: proto.RoleUser, Content: "question"}, {Role: proto.RoleAssistant, Content: "answer"}}
	require.NoError(t, conversations.Write("show-id", &messages))

	m := &Mods{cache: conversations, Config: &Config{Output: "json", cacheReadFromID: "show-id"}}
	require.NoError(t, m.readFromCache())
	require.Equal(t, messages, m.messages)
	require.Contains(t, m.Output, "question")
	require.Contains(t, m.Output, "answer")
}

func TestEnsureKeyPriority(t *testing.T) {
	m := Mods{}
	const docsURL = "https://example.com"
	t.Setenv("MY_KEY_ENV", "env-val")
	key, err := m.ensureKey(API{APIKey: "explicit", APIKeyEnv: "MY_KEY_ENV", APIKeyCmd: "echo cmd-val"}, "UNSET_XYZ", docsURL)
	require.NoError(t, err)
	require.Equal(t, "cmd-val", key)
	key, err = m.ensureKey(API{APIKey: "explicit", APIKeyEnv: "MY_KEY_ENV"}, "UNSET_XYZ", docsURL)
	require.NoError(t, err)
	require.Equal(t, "env-val", key)
	_, err = m.ensureKey(API{}, "UNSET_MODS_KEY_XYZ", docsURL)
	require.Error(t, err)
}

func TestFindCacheOpsDetails(t *testing.T) {
	newTestMods := func(t *testing.T) *Mods { return &Mods{db: testDB(t), Config: &Config{}} }
	t.Run("new conversation", func(t *testing.T) {
		details, err := newTestMods(t).findCacheOpsDetails()
		require.NoError(t, err)
		require.NotEmpty(t, details.WriteID)
	})
	t.Run("continue resolves saved conversation", func(t *testing.T) {
		m := newTestMods(t)
		id := newConversationID()
		require.NoError(t, m.db.Save(id, "message", "openai", "gpt-4"))
		m.Config.Continue = id[:8]
		m.Config.Prefix = "prompt"
		details, err := m.findCacheOpsDetails()
		require.NoError(t, err)
		require.Equal(t, id, details.ReadID)
		require.Equal(t, id, details.WriteID)
	})
	t.Run("unknown show reports a user error", func(t *testing.T) {
		m := newTestMods(t)
		m.Config.Show = "missing"
		_, err := m.findCacheOpsDetails()
		require.Error(t, err)
		var merr modsError
		require.True(t, errors.As(err, &merr))
		require.Equal(t, "Could not find the conversation.", merr.Reason())
	})
}

func TestResolveModel(t *testing.T) {
	m := &Mods{}
	cfg := &Config{APIs: APIs{
		{Name: "openai", Models: map[string]Model{"gpt": {Aliases: []string{"g"}}}},
		{Name: "anthropic", Models: map[string]Model{"sonnet": {Aliases: []string{"s"}}}},
	}}
	cfg.Model = "s"
	api, mod, err := m.resolveModel(cfg)
	require.NoError(t, err)
	require.Equal(t, "anthropic", api.Name)
	require.Equal(t, "sonnet", mod.Name)

	cfg.API, cfg.Model = "openai", "s"
	_, _, err = m.resolveModel(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "-a anthropic")
}

func TestCutPrompt(t *testing.T) {
	msg := fmt.Sprintf("This model's maximum context length is %d tokens. However, your messages resulted in %d tokens", 10, 10)
	require.Equal(t, "abcdefghij", cutPrompt(msg, "abcdefghijklmnopqrst"))
	require.Equal(t, "prompt", cutPrompt("other", "prompt"))
}
