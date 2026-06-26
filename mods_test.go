package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
	"github.com/stretchr/testify/require"
)

// fakeToolCallStream is a stream.Stream that is always finished and always
// returns the provided tool call statuses. Used to drive receiveCompletionStreamCmd.
type fakeToolCallStream struct {
	calls []proto.ToolCallStatus
}

func (f *fakeToolCallStream) Next() bool                           { return false }
func (f *fakeToolCallStream) Current() (proto.Chunk, error)       { return proto.Chunk{}, nil }
func (f *fakeToolCallStream) Close() error                         { return nil }
func (f *fakeToolCallStream) Err() error                           { return nil }
func (f *fakeToolCallStream) Messages() []proto.Message            { return nil }
func (f *fakeToolCallStream) CallTools() []proto.ToolCallStatus    { return f.calls }

var _ stream.Stream = (*fakeToolCallStream)(nil)

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

func TestEnsureKeyPriority(t *testing.T) {
	m := Mods{}
	const docsURL = "https://example.com"

	t.Run("explicit api-key wins over env and cmd", func(t *testing.T) {
		t.Setenv("MY_KEY_ENV", "env-val")
		key, err := m.ensureKey(API{
			APIKey:    "explicit-key",
			APIKeyEnv: "MY_KEY_ENV",
			APIKeyCmd: "echo cmd-val",
		}, "UNSET_XYZ", docsURL)
		require.NoError(t, err)
		require.Equal(t, "explicit-key", key)
	})

	t.Run("api-key-env wins over api-key-cmd", func(t *testing.T) {
		t.Setenv("MY_KEY_ENV", "env-val")
		key, err := m.ensureKey(API{
			APIKeyEnv: "MY_KEY_ENV",
			APIKeyCmd: "echo cmd-val",
		}, "UNSET_XYZ", docsURL)
		require.NoError(t, err)
		require.Equal(t, "env-val", key)
	})

	t.Run("api-key-cmd used when env is absent", func(t *testing.T) {
		key, err := m.ensureKey(API{
			APIKeyCmd: "echo cmd-val",
		}, "UNSET_XYZ", docsURL)
		require.NoError(t, err)
		require.Equal(t, "cmd-val", key)
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
