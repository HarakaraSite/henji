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

func TestMessagesForCacheOmitsImageDataAndConversationShowsMarker(t *testing.T) {
	messages := []Message{{
		Role:    RoleUser,
		Content: "describe this",
		Parts:   []ContentPart{{Type: ContentPartImage, Image: &Image{MediaType: "image/png", Data: []byte("secret-image")}}},
	}}

	cached := MessagesForCache(messages)
	require.Len(t, cached[0].Parts, 1)
	require.True(t, cached[0].Parts[0].ImageOmitted)
	require.Nil(t, cached[0].Parts[0].Image)
	require.NotContains(t, Conversation(cached).String(), "secret-image")
	require.Contains(t, Conversation(cached).String(), "[image omitted from saved conversation]")
}
