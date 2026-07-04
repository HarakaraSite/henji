package openai

import (
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
			messages = append(messages, openai.UserMessage(msg.Content))
		case proto.RoleAssistant:
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}
	return messages
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
