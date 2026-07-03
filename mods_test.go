package main

import (
	"fmt"
	"sync"
	"testing"

	"forge.harakara.site/littleisland/henji/internal/proto"
	"forge.harakara.site/littleisland/henji/internal/stream"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// fakeToolCallStream is a stream.Stream that is always finished and always
// returns the provided tool call statuses. Used to drive receiveCompletionStreamCmd.
type fakeToolCallStream struct {
	calls []proto.ToolCallStatus
}

func (f *fakeToolCallStream) Next() bool                        { return false }
func (f *fakeToolCallStream) Current() (proto.Chunk, error)     { return proto.Chunk{}, nil }
func (f *fakeToolCallStream) Close() error                      { return nil }
func (f *fakeToolCallStream) Err() error                        { return nil }
func (f *fakeToolCallStream) Messages() []proto.Message         { return nil }
func (f *fakeToolCallStream) CallTools() []proto.ToolCallStatus { return f.calls }

var _ stream.Stream = (*fakeToolCallStream)(nil)

// fakeMessagesStream is a finished stream.Stream with no pending tool calls,
// used to drive the --json-schema validation path in receiveCompletionStreamCmd.
type fakeMessagesStream struct {
	messages []proto.Message
}

func (f *fakeMessagesStream) Next() bool                        { return false }
func (f *fakeMessagesStream) Current() (proto.Chunk, error)     { return proto.Chunk{}, nil }
func (f *fakeMessagesStream) Close() error                      { return nil }
func (f *fakeMessagesStream) Err() error                        { return nil }
func (f *fakeMessagesStream) Messages() []proto.Message         { return f.messages }
func (f *fakeMessagesStream) CallTools() []proto.ToolCallStatus { return nil }

var _ stream.Stream = (*fakeMessagesStream)(nil)

func TestMaxToolCallsLimit(t *testing.T) {
	fs := &fakeToolCallStream{
		calls: []proto.ToolCallStatus{{Name: "search"}},
	}
	errh := func(err error) tea.Msg { return modsError{err: err} }

	t.Run("allows rounds up to the limit", func(t *testing.T) {
		m := &Mods{Config: &Config{MaxToolCalls: 2}}

		// round 1 (toolCallRound 0→1): within limit
		result := m.receiveCompletionStreamCmd(completionOutput{
			stream: fs, errh: errh, toolCallRound: 0,
		})()
		out, ok := result.(completionOutput)
		require.True(t, ok, "round 1: expected completionOutput, got %T", result)
		require.Equal(t, 1, out.toolCallRound)

		// round 2 (toolCallRound 1→2): at the limit, still allowed
		result = m.receiveCompletionStreamCmd(completionOutput{
			stream: fs, errh: errh, toolCallRound: 1,
		})()
		out, ok = result.(completionOutput)
		require.True(t, ok, "round 2: expected completionOutput, got %T", result)
		require.Equal(t, 2, out.toolCallRound)
	})

	t.Run("stops when exceeding the limit", func(t *testing.T) {
		m := &Mods{Config: &Config{MaxToolCalls: 2}}

		// round 3 (nextRound=3 > limit=2): must stop
		result := m.receiveCompletionStreamCmd(completionOutput{
			stream: fs, errh: errh, toolCallRound: 2,
		})()
		err, ok := result.(modsError)
		require.True(t, ok, "expected modsError, got %T", result)
		require.Contains(t, err.Error(), "tool call limit of 2 reached")
	})

	t.Run("zero means unlimited", func(t *testing.T) {
		m := &Mods{Config: &Config{MaxToolCalls: 0}}

		for i := range 5 {
			result := m.receiveCompletionStreamCmd(completionOutput{
				stream: fs, errh: errh, toolCallRound: i,
			})()
			out, ok := result.(completionOutput)
			require.True(t, ok, "round %d: expected completionOutput, got %T", i+1, result)
			require.Equal(t, i+1, out.toolCallRound)
		}
	})
}

func TestCheckJSONSchemaValidResponsePassesThrough(t *testing.T) {
	_, schema, err := loadJSONSchema(writeTempSchema(t, `{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`))
	require.NoError(t, err)

	fs := &fakeMessagesStream{messages: []proto.Message{
		{Role: proto.RoleUser, Content: "hi"},
		{Role: proto.RoleAssistant, Content: `{"ok":true}`},
	}}
	m := &Mods{Config: &Config{jsonSchemaValidator: schema, JSONSchemaRetries: 2}, contentMutex: &sync.Mutex{}}
	errh := func(err error) tea.Msg { return modsError{err: err} }

	result := m.receiveCompletionStreamCmd(completionOutput{stream: fs, errh: errh})()
	_, ok := result.(completionOutput)
	require.True(t, ok, "expected completionOutput, got %T", result)
	require.Equal(t, 0, m.schemaRetries)
}

// TestCheckJSONSchemaInvalidResponseTriggersRetryWithCorrection is a
// regression test: unlike m.retry (which resets the whole conversation via
// setupStreamContext), a schema validation failure must keep the failed
// assistant turn in m.messages and hand a correction message to retryFn.
func TestCheckJSONSchemaInvalidResponseTriggersRetryWithCorrection(t *testing.T) {
	_, schema, err := loadJSONSchema(writeTempSchema(t, `{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`))
	require.NoError(t, err)

	fs := &fakeMessagesStream{messages: []proto.Message{
		{Role: proto.RoleUser, Content: "hi"},
		{Role: proto.RoleAssistant, Content: `not json`},
	}}
	m := &Mods{Config: &Config{jsonSchemaValidator: schema, JSONSchemaRetries: 2}, contentMutex: &sync.Mutex{}}

	var gotCorrection string
	retryFn := func(correction string) tea.Msg {
		gotCorrection = correction
		return completionOutput{content: "retried"}
	}
	errh := func(err error) tea.Msg { return modsError{err: err} }

	result := m.receiveCompletionStreamCmd(completionOutput{stream: fs, errh: errh, retryFn: retryFn})()

	require.Equal(t, 1, m.schemaRetries)
	require.Contains(t, gotCorrection, "did not conform")
	out, ok := result.(completionOutput)
	require.True(t, ok, "expected completionOutput from retryFn, got %T", result)
	require.Equal(t, "retried", out.content)
	require.Len(t, m.messages, 2, "the failed assistant turn must not be wiped from context")
}

// TestCheckJSONSchemaRetryDiscardsFailedOutput is a regression test:
// appendToOutput accumulates into m.Output unconditionally (it only guards
// whether chunks are also live-printed), so a failed attempt's text was
// being concatenated with the retried response instead of being replaced by
// it. checkJSONSchema must clear m.Output/m.content before handing off to
// retryFn.
func TestCheckJSONSchemaRetryDiscardsFailedOutput(t *testing.T) {
	_, schema, err := loadJSONSchema(writeTempSchema(t, `{"type":"object","required":["ok"]}`))
	require.NoError(t, err)

	m := &Mods{
		Config:       &Config{jsonSchemaValidator: schema, JSONSchemaRetries: 2},
		contentMutex: &sync.Mutex{},
	}
	// Simulate the failed stream's chunks having already been rendered, the
	// same way Update() calls appendToOutput as each chunk arrives.
	m.appendToOutput("not json")
	require.NotEmpty(t, m.Output, "test setup: appendToOutput should have populated m.Output")

	fs := &fakeMessagesStream{messages: []proto.Message{
		{Role: proto.RoleUser, Content: "hi"},
		{Role: proto.RoleAssistant, Content: "not json"},
	}}
	retryFn := func(string) tea.Msg { return completionOutput{content: "retried"} }
	errh := func(err error) tea.Msg { return modsError{err: err} }

	m.receiveCompletionStreamCmd(completionOutput{stream: fs, errh: errh, retryFn: retryFn})()

	require.Empty(t, m.Output, "the discarded attempt's output must not survive into the retry")
	require.Empty(t, m.content)
}

func TestCheckJSONSchemaFailsFastAfterRetriesExhausted(t *testing.T) {
	_, schema, err := loadJSONSchema(writeTempSchema(t, `{"type":"object","required":["ok"]}`))
	require.NoError(t, err)

	fs := &fakeMessagesStream{messages: []proto.Message{
		{Role: proto.RoleAssistant, Content: `not json`},
	}}
	m := &Mods{Config: &Config{jsonSchemaValidator: schema, JSONSchemaRetries: 1}, schemaRetries: 1, contentMutex: &sync.Mutex{}}
	retryFn := func(string) tea.Msg {
		t.Fatal("retryFn must not be called once retries are exhausted")
		return nil
	}
	errh := func(err error) tea.Msg { return modsError{err: err} }

	result := m.receiveCompletionStreamCmd(completionOutput{stream: fs, errh: errh, retryFn: retryFn})()

	msgErr, ok := result.(modsError)
	require.True(t, ok, "expected modsError, got %T", result)
	require.Contains(t, msgErr.Reason(), "did not match --json-schema")
}

func TestEnsureKeyPriority(t *testing.T) {
	m := Mods{}
	const docsURL = "https://example.com"

	t.Run("api-key-cmd wins over env and explicit api-key", func(t *testing.T) {
		t.Setenv("MY_KEY_ENV", "env-val")
		key, err := m.ensureKey(API{
			APIKey:    "explicit-key",
			APIKeyEnv: "MY_KEY_ENV",
			APIKeyCmd: "echo cmd-val",
		}, "UNSET_XYZ", docsURL)
		require.NoError(t, err)
		require.Equal(t, "cmd-val", key)
	})

	t.Run("api-key-env wins over explicit api-key when cmd is absent", func(t *testing.T) {
		t.Setenv("MY_KEY_ENV", "env-val")
		key, err := m.ensureKey(API{
			APIKey:    "explicit-key",
			APIKeyEnv: "MY_KEY_ENV",
		}, "UNSET_XYZ", docsURL)
		require.NoError(t, err)
		require.Equal(t, "env-val", key)
	})

	t.Run("explicit api-key used when cmd and env are absent", func(t *testing.T) {
		key, err := m.ensureKey(API{
			APIKey: "explicit-key",
		}, "UNSET_XYZ", docsURL)
		require.NoError(t, err)
		require.Equal(t, "explicit-key", key)
	})

	t.Run("default env used as last resort", func(t *testing.T) {
		t.Setenv("FALLBACK_KEY", "fallback-val")
		key, err := m.ensureKey(API{}, "FALLBACK_KEY", docsURL)
		require.NoError(t, err)
		require.Equal(t, "fallback-val", key)
	})

	t.Run("error when all sources are empty", func(t *testing.T) {
		_, err := m.ensureKey(API{}, "UNSET_MODS_KEY_XYZ", docsURL)
		require.Error(t, err)
	})

	// A configured api-key-cmd is authoritative: if it fails, ensureKey must
	// not silently fall back to a weaker source (explicit api-key, env, or
	// the provider default env var) that happens to also be set.
	t.Run("api-key-cmd failure does not fall back to a weaker source", func(t *testing.T) {
		t.Setenv("FALLBACK_KEY", "fallback-val")
		_, err := m.ensureKey(API{
			APIKey:    "explicit-key",
			APIKeyCmd: "false", // exits non-zero, produces no output
		}, "FALLBACK_KEY", docsURL)
		require.Error(t, err)
	})
}

func TestFindCacheOpsDetails(t *testing.T) {
	newMods := func(t *testing.T) *Mods {
		db := testDB(t)
		return &Mods{
			db:     db,
			Config: &Config{},
		}
	}

	t.Run("all empty", func(t *testing.T) {
		msg := newMods(t).findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Empty(t, dets.ReadID)
		require.NotEmpty(t, dets.WriteID)
		require.Empty(t, dets.Title)
	})

	t.Run("show id", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message", "openai", "gpt-4"))
		mods.Config.Show = id[:8]
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
	})

	t.Run("show title", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message 1", "openai", "gpt-4"))
		mods.Config.Show = "message 1"
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
	})

	t.Run("continue id", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message", "openai", "gpt-4"))
		mods.Config.Continue = id[:5]
		mods.Config.Prefix = "prompt"
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
		require.Equal(t, id, dets.WriteID)
	})

	t.Run("continue with no prompt", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message 1", "openai", "gpt-4"))
		mods.Config.ContinueLast = true
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
		require.Equal(t, id, dets.WriteID)
		require.Empty(t, dets.Title)
	})

	t.Run("continue title", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message 1", "openai", "gpt-4"))
		mods.Config.Continue = "message 1"
		mods.Config.Prefix = "prompt"
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
		require.Equal(t, id, dets.WriteID)
	})

	t.Run("continue last", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message 1", "openai", "gpt-4"))
		mods.Config.ContinueLast = true
		mods.Config.Prefix = "prompt"
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
		require.Equal(t, id, dets.WriteID)
		require.Empty(t, dets.Title)
	})

	t.Run("continue last with name", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message 1", "openai", "gpt-4"))
		mods.Config.Continue = "message 2"
		mods.Config.Prefix = "prompt"
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
		require.Equal(t, "message 2", dets.Title)
		require.NotEmpty(t, dets.WriteID)
		require.Equal(t, id, dets.WriteID)
	})

	t.Run("write", func(t *testing.T) {
		mods := newMods(t)
		mods.Config.Title = "some title"
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Empty(t, dets.ReadID)
		require.NotEmpty(t, dets.WriteID)
		require.NotEqual(t, "some title", dets.WriteID)
		require.Equal(t, "some title", dets.Title)
	})

	t.Run("continue id and write with title", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message 1", "openai", "gpt-4"))
		mods.Config.Title = "some title"
		mods.Config.Continue = id[:10]
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
		require.NotEmpty(t, dets.WriteID)
		require.NotEqual(t, id, dets.WriteID)
		require.NotEqual(t, "some title", dets.WriteID)
		require.Equal(t, "some title", dets.Title)
	})

	t.Run("continue title and write with title", func(t *testing.T) {
		mods := newMods(t)
		id := newConversationID()
		require.NoError(t, mods.db.Save(id, "message 1", "openai", "gpt-4"))
		mods.Config.Title = "some title"
		mods.Config.Continue = "message 1"
		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)
		require.Equal(t, id, dets.ReadID)
		require.NotEmpty(t, dets.WriteID)
		require.NotEqual(t, id, dets.WriteID)
		require.NotEqual(t, "some title", dets.WriteID)
		require.Equal(t, "some title", dets.Title)
	})

	t.Run("show invalid", func(t *testing.T) {
		mods := newMods(t)
		mods.Config.Show = "aaa"
		msg := mods.findCacheOpsDetails()()
		err := msg.(modsError)
		require.Equal(t, "Could not find the conversation.", err.reason)
		require.EqualError(t, err, "no conversations found: aaa")
	})

	t.Run("uses config model and api not global config", func(t *testing.T) {
		mods := newMods(t)
		mods.Config.Model = "claude-3.7-sonnet"
		mods.Config.API = "anthropic"

		msg := mods.findCacheOpsDetails()()
		dets := msg.(cacheDetailsMsg)

		require.Equal(t, "claude-3.7-sonnet", dets.Model)
		require.Equal(t, "anthropic", dets.API)
		require.Empty(t, dets.ReadID)
		require.NotEmpty(t, dets.WriteID)
	})
}

func TestRemoveWhitespace(t *testing.T) {
	t.Run("only whitespaces", func(t *testing.T) {
		require.Equal(t, "", removeWhitespace(" \n"))
	})

	t.Run("regular text", func(t *testing.T) {
		require.Equal(t, " regular\n ", removeWhitespace(" regular\n "))
	})
}

func TestPromptExcerpt(t *testing.T) {
	t.Run("zero disables prompt echo", func(t *testing.T) {
		require.Equal(t, "", promptExcerpt("line 1\nline 2", 0))
	})

	t.Run("positive includes requested lines", func(t *testing.T) {
		require.Equal(t, "line 1\nline 2\n", promptExcerpt("line 1\nline 2\nline 3", 2))
	})

	t.Run("negative includes all input", func(t *testing.T) {
		require.Equal(t, "line 1\nline 2\nline 3\n", promptExcerpt("line 1\nline 2\nline 3", -1))
	})

	t.Run("empty input stays empty", func(t *testing.T) {
		require.Equal(t, "", promptExcerpt("", -1))
	})
}

func TestResolveModel(t *testing.T) {
	mods := &Mods{}
	newConfig := func() *Config {
		return &Config{
			APIs: APIs{
				{
					Name: "openai",
					Models: map[string]Model{
						"gpt-4o": {
							Aliases:  []string{"4o"},
							MaxChars: 100,
							Fallback: "gpt-4o-mini",
						},
						"gpt-4o-mini": {
							Aliases:  []string{"mini"},
							MaxChars: 50,
						},
					},
				},
				{
					Name: "anthropic",
					Models: map[string]Model{
						"claude-sonnet": {
							Aliases:  []string{"sonnet"},
							MaxChars: 200,
						},
					},
				},
			},
		}
	}

	t.Run("resolves alias across configured APIs", func(t *testing.T) {
		cfg := newConfig()
		cfg.Model = "sonnet"

		api, mod, err := mods.resolveModel(cfg)

		require.NoError(t, err)
		require.Equal(t, "anthropic", api.Name)
		require.Equal(t, "claude-sonnet", cfg.Model)
		require.Equal(t, "claude-sonnet", mod.Name)
		require.Equal(t, "anthropic", mod.API)
		require.EqualValues(t, 200, mod.MaxChars)
	})

	t.Run("respects explicit api", func(t *testing.T) {
		cfg := newConfig()
		cfg.API = "openai"
		cfg.Model = "4o"

		api, mod, err := mods.resolveModel(cfg)

		require.NoError(t, err)
		require.Equal(t, "openai", api.Name)
		require.Equal(t, "gpt-4o", cfg.Model)
		require.Equal(t, "gpt-4o", mod.Name)
		require.Equal(t, "openai", mod.API)
		require.Equal(t, "gpt-4o-mini", mod.Fallback)
	})

	t.Run("errors when explicit api does not contain model", func(t *testing.T) {
		cfg := newConfig()
		cfg.API = "openai"
		cfg.Model = "sonnet"

		_, _, err := mods.resolveModel(cfg)

		require.Error(t, err)
		var merr modsError
		require.ErrorAs(t, err, &merr)
		require.Contains(t, merr.reason, "does not contain")
	})
}

var cutPromptTests = map[string]struct {
	msg      string
	prompt   string
	expected string
}{
	"bad error": {
		msg:      "nope",
		prompt:   "the prompt",
		expected: "the prompt",
	},
	"crazy error": {
		msg:      tokenErrMsg(10, 93),
		prompt:   "the prompt",
		expected: "the prompt",
	},
	"cut prompt": {
		msg:      tokenErrMsg(10, 3),
		prompt:   "this is a long prompt I have no idea if its really 10 tokens",
		expected: "this is a long prompt ",
	},
	"missmatch of token estimation vs api result": {
		msg:      tokenErrMsg(30000, 100),
		prompt:   "tell me a joke",
		expected: "tell me a joke",
	},
}

func tokenErrMsg(l, ml int) string {
	return fmt.Sprintf(`This model's maximum context length is %d tokens. However, your messages resulted in %d tokens`, ml, l)
}

func TestCutPrompt(t *testing.T) {
	for name, tc := range cutPromptTests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, cutPrompt(tc.msg, tc.prompt))
		})
	}
}
