package anthropic

import (
	"encoding/base64"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/anthropics/anthropic-sdk-go"
)

func fromProtoMessages(input []proto.Message) (system []anthropic.TextBlockParam, messages []anthropic.MessageParam) {
	for _, msg := range input {
		switch msg.Role {
		case proto.RoleSystem:
			// system is not a role in anthropic, must set it as the system part of the request.
			system = append(system, *anthropic.NewTextBlock(msg.Content).OfText)
		case proto.RoleUser:
			if len(msg.Parts) == 0 {
				messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
				continue
			}
			parts := anthropicContentParts(msg.Parts)
			if len(parts) == 0 {
				messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
				continue
			}
			messages = append(messages, anthropic.NewUserMessage(parts...))
		case proto.RoleAssistant:
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}
	return system, messages
}

func anthropicContentParts(parts []proto.ContentPart) []anthropic.ContentBlockParamUnion {
	result := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for _, part := range parts {
		switch {
		case part.Type == proto.ContentPartText:
			result = append(result, anthropic.NewTextBlock(part.Text))
		case part.Image != nil:
			result = append(result, anthropic.NewImageBlockBase64(part.Image.MediaType, base64.StdEncoding.EncodeToString(part.Image.Data)))
		}
	}
	return result
}

func toProtoMessage(in anthropic.MessageParam) proto.Message {
	msg := proto.Message{
		Role: string(in.Role),
	}

	for _, block := range in.Content {
		if txt := block.OfText; txt != nil {
			msg.Content += txt.Text
		}
	}

	return msg
}
