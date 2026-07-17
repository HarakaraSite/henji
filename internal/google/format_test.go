package google

import (
	"encoding/json"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestFromProtoMessagesEncodesInlineImageData(t *testing.T) {
	contents := fromProtoMessages([]proto.Message{{
		Role: proto.RoleUser,
		Parts: []proto.ContentPart{
			{Type: proto.ContentPartText, Text: "describe it"},
			{Type: proto.ContentPartImage, Image: &proto.Image{MediaType: "image/png", Data: []byte("png")}},
		},
	}})
	body, err := json.Marshal(contents)
	require.NoError(t, err)
	require.JSONEq(t, `[{"parts":[{"text":"describe it"},{"inline_data":{"mime_type":"image/png","data":"cG5n"}}],"role":"user"}]`, string(body))
}

func TestFromProtoMessagesSkipsCachedTextAttachmentMarker(t *testing.T) {
	contents := fromProtoMessages([]proto.Message{{
		Role:    proto.RoleUser,
		Content: "previous prompt",
		Parts:   []proto.ContentPart{{Type: proto.ContentPartText, TextOmitted: true}},
	}})
	body, err := json.Marshal(contents)
	require.NoError(t, err)
	require.JSONEq(t, `[{"parts":[{"text":"previous prompt"}],"role":"user"}]`, string(body))
}
