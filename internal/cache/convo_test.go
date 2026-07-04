package cache

import (
	"bytes"
	"encoding/gob"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

// legacyMessage, legacyToolCall, and legacyFunction mirror the exact wire
// shape proto.Message had while MCP support existed (Role/Content plus a
// ToolCalls slice). Conversations saved back then are still sitting in
// users' conversation caches after the MCP removal.
type legacyMessage struct {
	Role      string
	Content   string
	ToolCalls []legacyToolCall
}

type legacyToolCall struct {
	ID       string
	Function legacyFunction
	IsError  bool
}

type legacyFunction struct {
	Name      string
	Arguments []byte
}

// TestDecodeLegacyMCPToolMessageDoesNotLeakContent is a regression test: a
// conversation saved before MCP was removed may contain a Role: "tool"
// message whose Content is a tool result payload (potentially including
// secrets). gob silently drops the now-unknown ToolCalls field on decode,
// but the Role/Content of that message survive intact. Displaying that
// message (e.g. via `henji --show`, which renders proto.Conversation.String())
// must never print the raw secret payload.
func TestDecodeLegacyMCPToolMessageDoesNotLeakContent(t *testing.T) {
	const secretMarker = "API_TOKEN=legacy-secret-value"

	legacy := []legacyMessage{
		{Role: proto.RoleUser, Content: "read the config file"},
		{
			Role:    "tool",
			Content: secretMarker,
			ToolCalls: []legacyToolCall{
				{
					ID: "call-1",
					Function: legacyFunction{
						Name:      "filesystem_read_file",
						Arguments: []byte(`{"path":"/etc/secret"}`),
					},
				},
			},
		},
		{Role: proto.RoleAssistant, Content: "done reading"},
	}

	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(&legacy))

	var messages []proto.Message
	require.NoError(t, decode(&buf, &messages))

	// The legacy tool message's Role/Content survive the decode (this is
	// what makes the display-time fix necessary rather than optional).
	require.Len(t, messages, 3)
	require.Equal(t, "tool", messages[1].Role)
	require.Equal(t, secretMarker, messages[1].Content)

	// Rendering the conversation (the --show path) must never surface the
	// secret payload, while normal messages still display as before.
	rendered := proto.Conversation(messages).String()
	require.NotContains(t, rendered, secretMarker)
	require.Contains(t, rendered, "read the config file")
	require.Contains(t, rendered, "done reading")
}
