package main

import (
	"testing"

	"forge.harakara.site/littleisland/henji/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestLastPrompt(t *testing.T) {
	t.Run("no prompt", func(t *testing.T) {
		require.Equal(t, "", lastPrompt(nil))
	})

	t.Run("single prompt", func(t *testing.T) {
		require.Equal(t, "single", lastPrompt([]proto.Message{
			{
				Role:    proto.RoleUser,
				Content: "single",
			},
		}))
	})

	t.Run("multiple prompts", func(t *testing.T) {
		require.Equal(t, "last", lastPrompt([]proto.Message{
			{
				Role:    proto.RoleUser,
				Content: "first",
			},
			{
				Role:    proto.RoleAssistant,
				Content: "hallo",
			},
			{
				Role:    proto.RoleUser,
				Content: "middle 1",
			},
			{
				Role:    proto.RoleUser,
				Content: "middle 2",
			},
			{
				Role:    proto.RoleUser,
				Content: "last",
			},
		}))
	})
}

func TestFirstLine(t *testing.T) {
	t.Run("single line", func(t *testing.T) {
		require.Equal(t, "line", firstLine("line"))
	})
	t.Run("single line ending with \n", func(t *testing.T) {
		require.Equal(t, "line", firstLine("line\n"))
	})
	t.Run("multiple lines", func(t *testing.T) {
		require.Equal(t, "line", firstLine("line\nsomething else\nline3\nfoo\nends with a double \n\n"))
	})
}

func TestConversationTitle(t *testing.T) {
	messages := []proto.Message{
		{
			Role:    proto.RoleUser,
			Content: "prompt title\nsecond line",
		},
	}

	t.Run("uses explicit title", func(t *testing.T) {
		require.Equal(t, "custom title", conversationTitle(" custom title ", messages))
	})

	t.Run("uses prompt first line when title is empty", func(t *testing.T) {
		require.Equal(t, "prompt title", conversationTitle("", messages))
	})

	t.Run("uses prompt first line when title is sha", func(t *testing.T) {
		require.Equal(t, "prompt title", conversationTitle("df31ae23ab8b75b5643c2f846c570997edc71333", messages))
	})

	t.Run("falls back when there is no prompt", func(t *testing.T) {
		require.Equal(t, defaultConversationTitle, conversationTitle("", nil))
	})

	t.Run("falls back when prompt first line is blank", func(t *testing.T) {
		require.Equal(t, defaultConversationTitle, conversationTitle("", []proto.Message{
			{
				Role:    proto.RoleUser,
				Content: "\nsecond line",
			},
		}))
	})
}
