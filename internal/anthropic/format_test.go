package anthropic

import (
	"encoding/json"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestFromProtoMessagesEncodesImageBlock(t *testing.T) {
	_, messages := fromProtoMessages([]proto.Message{{
		Role: proto.RoleUser,
		Parts: []proto.ContentPart{
			{Type: proto.ContentPartText, Text: "describe it"},
			{Type: proto.ContentPartImage, Image: &proto.Image{MediaType: "image/png", Data: []byte("png")}},
		},
	}})
	body, err := json.Marshal(messages[0])
	require.NoError(t, err)
	require.Contains(t, string(body), `"type":"image"`)
	require.Contains(t, string(body), `"media_type":"image/png"`)
	require.Contains(t, string(body), `"data":"cG5n"`)
}

func TestFromProtoMessagesSkipsCachedTextAttachmentMarker(t *testing.T) {
	_, messages := fromProtoMessages([]proto.Message{{
		Role:    proto.RoleUser,
		Content: "previous prompt",
		Parts:   []proto.ContentPart{{Type: proto.ContentPartText, TextOmitted: true}},
	}})
	body, err := json.Marshal(messages[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"user","content":[{"type":"text","text":"previous prompt"}]}`, string(body))
}
