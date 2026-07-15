package main

import (
	"os"
	"path/filepath"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/cache"
	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestSetupStreamContext(t *testing.T) {
	t.Run("builds system and user messages", func(t *testing.T) {
		roleFile := filepath.Join(t.TempDir(), "role.txt")
		require.NoError(t, writeTestFile(roleFile, "role from file"))

		mods := &Mods{
			Config: &Config{
				Format:   true,
				FormatAs: "json",
				FormatText: FormatText{
					"json": "format as json",
				},
				Role: "tester",
				Roles: map[string][]string{
					"tester": {
						"inline role",
						"file://" + roleFile,
					},
				},
				Prefix: "prefix prompt",
			},
		}

		err := mods.setupStreamContext("stdin prompt", Model{MaxChars: 100})

		require.NoError(t, err)
		require.Equal(t, []proto.Message{
			{Role: proto.RoleSystem, Content: "format as json"},
			{Role: proto.RoleSystem, Content: "inline role"},
			{Role: proto.RoleSystem, Content: "role from file"},
			{Role: proto.RoleUser, Content: "prefix prompt\n\nstdin prompt"},
		}, mods.messages)
	})

	t.Run("truncates user content by model max chars", func(t *testing.T) {
		mods := &Mods{
			Config: &Config{
				Prefix: "prefix",
			},
		}

		err := mods.setupStreamContext("1234567890", Model{MaxChars: 8})

		require.NoError(t, err)
		require.Equal(t, []proto.Message{
			{Role: proto.RoleUser, Content: "prefix\n\n"},
		}, mods.messages)
	})

	t.Run("keeps long user content with no limit", func(t *testing.T) {
		mods := &Mods{
			Config: &Config{
				NoLimit: true,
			},
		}

		err := mods.setupStreamContext("1234567890", Model{MaxChars: 3})

		require.NoError(t, err)
		require.Equal(t, []proto.Message{
			{Role: proto.RoleUser, Content: "1234567890"},
		}, mods.messages)
	})

	t.Run("keeps long user content when model max chars is unset", func(t *testing.T) {
		mods := &Mods{
			Config: &Config{},
		}

		err := mods.setupStreamContext("1234567890", Model{})

		require.NoError(t, err)
		require.Equal(t, []proto.Message{
			{Role: proto.RoleUser, Content: "1234567890"},
		}, mods.messages)
	})

	t.Run("loads cached conversation before new user message", func(t *testing.T) {
		convos, err := cache.NewConversations(t.TempDir())
		require.NoError(t, err)

		cached := []proto.Message{
			{Role: proto.RoleUser, Content: "old question"},
			{Role: proto.RoleAssistant, Content: "old answer"},
		}
		require.NoError(t, convos.Write("abc123", &cached))

		mods := &Mods{
			cache: convos,
			Config: &Config{
				cacheReadFromID: "abc123",
			},
		}

		err = mods.setupStreamContext("new question", Model{MaxChars: 100})

		require.NoError(t, err)
		require.Equal(t, []proto.Message{
			{Role: proto.RoleUser, Content: "old question"},
			{Role: proto.RoleAssistant, Content: "old answer"},
			{Role: proto.RoleUser, Content: "new question"},
		}, mods.messages)
	})

	t.Run("keeps image between text attachment and stdin", func(t *testing.T) {
		image := &proto.Image{MediaType: "image/png", Data: []byte("png")}
		mods := &Mods{
			inputParts: buildInputParts("requirements", image, "diff"),
			rawInput:   "requirements\n\ndiff",
			Config:     &Config{Prefix: "review"},
		}

		err := mods.setupStreamContext(mods.rawInput, Model{MaxChars: 100})
		require.NoError(t, err)
		message := mods.messages[0]
		require.Equal(t, "review\n\nrequirements\n\ndiff", message.Content)
		require.Len(t, message.Parts, 4)
		require.Equal(t, "review", message.Parts[0].Text)
		require.Equal(t, "requirements", message.Parts[1].Text)
		require.Same(t, image, message.Parts[2].Image)
		require.Equal(t, "diff", message.Parts[3].Text)
	})

	t.Run("keeps image between text attachment and shortened stdin on retry", func(t *testing.T) {
		image := &proto.Image{MediaType: "image/png", Data: []byte("png")}
		mods := &Mods{
			inputParts: buildInputParts("requirements", image, "diff output"),
			rawInput:   "requirements\n\ndiff output",
			Config:     &Config{},
		}

		err := mods.setupStreamContext("requirements\n\ndif", Model{MaxChars: 100})
		require.NoError(t, err)
		message := mods.messages[0]
		require.Equal(t, "requirements\n\ndif", message.Content)
		require.Len(t, message.Parts, 3)
		require.Equal(t, "requirements", message.Parts[0].Text)
		require.Same(t, image, message.Parts[1].Image)
		require.Equal(t, "dif", message.Parts[2].Text)
	})

	t.Run("returns error for missing role", func(t *testing.T) {
		mods := &Mods{
			Config: &Config{
				Role:  "missing",
				Roles: map[string][]string{},
			},
		}

		err := mods.setupStreamContext("prompt", Model{MaxChars: 100})

		require.Error(t, err)
		var merr modsError
		require.ErrorAs(t, err, &merr)
		require.Equal(t, "Could not use role", merr.reason)
	})
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
