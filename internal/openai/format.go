package openai

import (
	"encoding/base64"
	"fmt"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/openai/openai-go"
)

func fromProtoMessages(input []proto.Message) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion
	for _, msg := range input {
		switch msg.Role {
		case proto.RoleSystem:
			messages = append(messages, openai.SystemMessage(msg.Content))
		case proto.RoleUser:
			if len(msg.Parts) == 0 {
				messages = append(messages, openai.UserMessage(msg.Content))
				continue
			}
			parts := openAIContentParts(msg.Parts)
			if len(parts) == 0 {
				messages = append(messages, openai.UserMessage(msg.Content))
				continue
			}
			messages = append(messages, openai.UserMessage(parts))
		case proto.RoleAssistant:
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}
	return messages
}

func openAIContentParts(parts []proto.ContentPart) []openai.ChatCompletionContentPartUnionParam {
	result := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		switch {
		case part.Type == proto.ContentPartText && !part.TextOmitted:
			result = append(result, openai.ChatCompletionContentPartUnionParam{
				OfText: &openai.ChatCompletionContentPartTextParam{Text: part.Text},
			})
		case part.Image != nil:
			result = append(result, openai.ChatCompletionContentPartUnionParam{
				OfImageURL: &openai.ChatCompletionContentPartImageParam{
					ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
						URL: fmt.Sprintf("data:%s;base64,%s", part.Image.MediaType, base64.StdEncoding.EncodeToString(part.Image.Data)),
					},
				},
			})
		}
	}
	return result
}

func toProtoMessage(in openai.ChatCompletionMessageParamUnion) proto.Message {
	msg := proto.Message{
		Role: msgRole(in),
	}
	switch content := in.GetContent().AsAny().(type) {
	case *string:
		if content == nil || *content == "" {
			break
		}
		msg.Content = *content
	case *[]openai.ChatCompletionContentPartTextParam:
		if content == nil || len(*content) == 0 {
			break
		}
		for _, c := range *content {
			msg.Content += c.Text
		}
	}
	return msg
}

func msgRole(in openai.ChatCompletionMessageParamUnion) string {
	if in.OfSystem != nil {
		return proto.RoleSystem
	}
	if in.OfAssistant != nil {
		return proto.RoleAssistant
	}
	if in.OfUser != nil {
		return proto.RoleUser
	}
	return ""
}
