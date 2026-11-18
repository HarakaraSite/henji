package main

import (
	"strings"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
)

const defaultConversationTitle = "Untitled conversation"

func lastPrompt(messages []proto.Message) string {
	var result string
	for _, msg := range messages {
		if msg.Role != proto.RoleUser {
			continue
		}
		if msg.Content == "" {
			continue
		}
		result = msg.Content
	}
	return result
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}

func conversationTitle(title string, messages []proto.Message) string {
	title = strings.TrimSpace(title)
	if sha1reg.MatchString(title) || title == "" {
		title = firstLine(lastPrompt(messages))
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return defaultConversationTitle
	}
	return title
}
