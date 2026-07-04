// Package proto shared protocol.
package proto

import (
	"strings"
)

// Roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Chunk is a streaming chunk of text.
type Chunk struct {
	Content string
}

// Message is a message in the conversation.
type Message struct {
	Role    string
	Content string
}

// Request is a chat request.
type Request struct {
	Messages            []Message
	API                 string
	Model               string
	User                string
	Temperature         *float64
	TopP                *float64
	TopK                *int64
	Stop                []string
	MaxTokens           *int64
	MaxCompletionTokens *int64
	ResponseFormat      *string
	// JSONSchema, when set, asks the provider to constrain its output to
	// this JSON Schema. It takes precedence over ResponseFormat.
	JSONSchema map[string]any
}

// Conversation is a conversation.
type Conversation []Message

func (cc Conversation) String() string {
	var sb strings.Builder
	for _, msg := range cc {
		if msg.Content == "" {
			continue
		}
		switch msg.Role {
		case RoleSystem:
			sb.WriteString("**System**: ")
		case RoleUser:
			sb.WriteString("**User**: ")
		case RoleAssistant:
			sb.WriteString("**Assistant**: ")
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}
