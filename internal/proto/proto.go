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
	Parts   []ContentPart
}

// ContentPart is one ordered piece of a user message.
type ContentPart struct {
	Type          string
	Text          string
	Image         *Image
	OmitFromCache bool
	ImageOmitted  bool
	TextOmitted   bool
}

const (
	ContentPartText  = "text"
	ContentPartImage = "image"
)

// Image is an in-memory image attachment. Paths are intentionally never kept.
type Image struct {
	MediaType string
	Data      []byte
}

// MessagesForCache returns a copy that never persists attachment data.
func MessagesForCache(messages []Message) []Message {
	result := make([]Message, len(messages))
	for i, msg := range messages {
		result[i] = msg
		var markers []ContentPart
		var savedText []string
		rebuildContent := false
		hasTextParts := false
		for _, part := range msg.Parts {
			switch {
			case part.Image != nil || (part.Type == ContentPartImage && !part.ImageOmitted):
				markers = append(markers, ContentPart{Type: ContentPartImage, ImageOmitted: true})
				rebuildContent = true
			case part.OmitFromCache:
				markers = append(markers, ContentPart{Type: ContentPartText, TextOmitted: true})
				rebuildContent = true
				hasTextParts = true
			case part.ImageOmitted:
				markers = append(markers, ContentPart{Type: ContentPartImage, ImageOmitted: true})
			case part.TextOmitted:
				markers = append(markers, ContentPart{Type: ContentPartText, TextOmitted: true})
			case part.Type == ContentPartText && part.Text != "":
				savedText = append(savedText, part.Text)
				hasTextParts = true
			}
		}
		if rebuildContent && hasTextParts {
			result[i].Content = strings.Join(savedText, "\n\n")
		}
		result[i].Parts = markers
	}
	return result
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
		if msg.Content == "" && len(msg.Parts) == 0 {
			continue
		}
		switch msg.Role {
		case RoleSystem:
			sb.WriteString("**System**: ")
		case RoleUser:
			sb.WriteString("**User**: ")
		case RoleAssistant:
			sb.WriteString("**Assistant**: ")
		default:
			// Unrecognized roles are skipped rather than printed raw. In
			// particular, conversations saved before MCP support was removed
			// may still contain Role: "tool" messages whose Content held a
			// tool result payload (potentially including secrets) that was
			// never meant to be displayed unlabeled.
			continue
		}
		sb.WriteString(msg.Content)
		for _, part := range msg.Parts {
			switch {
			case part.TextOmitted:
				sb.WriteString("\n[text attachment omitted from saved conversation]")
			case part.ImageOmitted:
				sb.WriteString("\n[image omitted from saved conversation]")
			}
		}
		sb.WriteString("\n\n")
	}
	return sb.String()
}
