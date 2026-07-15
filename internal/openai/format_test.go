package openai

import (
	"encoding/json"
	"testing"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestFromProtoMessagesEncodesImageDataURL(t *testing.T) {
	messages := fromProtoMessages([]proto.Message{{
		Role:    proto.RoleUser,
		Content: "describe it",
		Parts: []proto.ContentPart{
			{Type: proto.ContentPartText, Text: "describe it"},
			{Type: proto.ContentPartImage, Image: &proto.Image{MediaType: "image/png", Data: []byte("png")}},
		},
	}})

	body, err := json.Marshal(messages[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"user","content":[{"type":"text","text":"describe it"},{"type":"image_url","image_url":{"url":"data:image/png;base64,cG5n"}}]}`, string(body))
}

func TestFromProtoMessagesSkipsCachedImageMarker(t *testing.T) {
	messages := fromProtoMessages([]proto.Message{{
		Role:    proto.RoleUser,
		Content: "previous image",
		Parts:   []proto.ContentPart{{Type: proto.ContentPartImage, ImageOmitted: true}},
	}})
	body, err := json.Marshal(messages[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"role":"user","content":"previous image"}`, string(body))
}

func TestFromProtoMessagesContinueOmitsPastImageAndSendsNewImage(t *testing.T) {
	messages := fromProtoMessages([]proto.Message{
		{
			Role:    proto.RoleUser,
			Content: "old request",
			Parts:   []proto.ContentPart{{Type: proto.ContentPartImage, ImageOmitted: true}},
		},
		{
			Role:    proto.RoleUser,
			Content: "new request",
			Parts: []proto.ContentPart{
				{Type: proto.ContentPartText, Text: "new request"},
				{Type: proto.ContentPartImage, Image: &proto.Image{MediaType: "image/png", Data: []byte("new-image")}},
			},
		},
	})

	body, err := json.Marshal(messages)
	require.NoError(t, err)
	require.NotContains(t, string(body), "image omitted")
	require.Contains(t, string(body), "bmV3LWltYWdl")
	require.JSONEq(t, `[
		{"role":"user","content":"old request"},
		{"role":"user","content":[
			{"type":"text","text":"new request"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,bmV3LWltYWdl"}}
		]}
	]`, string(body))
}
