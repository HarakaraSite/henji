package proto

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
)

func TestStringer(t *testing.T) {
	messages := []Message{
		{
			Role:    RoleSystem,
			Content: "you are a medieval king",
		},
		{
			Role:    RoleUser,
			Content: "first 4 natural numbers",
		},
		{
			Role:    RoleAssistant,
			Content: "1, 2, 3, 4",
		},
		{
			Role:    RoleUser,
			Content: "as a json array",
		},
		{
			Role:    RoleAssistant,
			Content: "[ 1, 2, 3, 4 ]",
		},
		{
			Role:    RoleAssistant,
			Content: "something from an assistant",
		},
	}

	golden.RequireEqual(t, []byte(Conversation(messages).String()))
}

func TestMessagesForCacheOmitsAttachmentDataAndConversationShowsMarkers(t *testing.T) {
	messages := []Message{{
		Role:    RoleUser,
		Content: "describe this\n\nsecret-text\n\nstdin",
		Parts: []ContentPart{
			{Type: ContentPartText, Text: "describe this"},
			{Type: ContentPartText, Text: "secret-text", OmitFromCache: true},
			{Type: ContentPartImage, Image: &Image{MediaType: "image/png", Data: []byte("secret-image")}},
			{Type: ContentPartText, Text: "stdin"},
		},
	}}

	cached := MessagesForCache(messages)
	require.Equal(t, "describe this\n\nstdin", cached[0].Content)
	require.Len(t, cached[0].Parts, 2)
	require.True(t, cached[0].Parts[0].TextOmitted)
	require.True(t, cached[0].Parts[1].ImageOmitted)
	require.Nil(t, cached[0].Parts[1].Image)
	require.NotContains(t, Conversation(cached).String(), "secret-text")
	require.NotContains(t, Conversation(cached).String(), "secret-image")
	require.Contains(t, Conversation(cached).String(), "[text attachment omitted from saved conversation]")
	require.Contains(t, Conversation(cached).String(), "[image omitted from saved conversation]")
}
