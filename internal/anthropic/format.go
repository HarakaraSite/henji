package anthropic

import (
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
			block := anthropic.NewTextBlock(msg.Content)
			messages = append(messages, anthropic.NewUserMessage(block))
		case proto.RoleAssistant:
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}
	return system, messages
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
