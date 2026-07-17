package main

import (
	"fmt"
	"strings"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
)

func (m *Mods) setupStreamContext(content string, mod Model) error {
	cfg := m.Config
	m.messages = []proto.Message{}
	if txt := cfg.FormatText[cfg.FormatAs]; cfg.Format && txt != "" {
		m.messages = append(m.messages, proto.Message{
			Role:    proto.RoleSystem,
			Content: txt,
		})
	}

	if cfg.Role != "" {
		roleSetup, ok := cfg.Roles[cfg.Role]
		if !ok {
			return modsError{
				err:    fmt.Errorf("role %q does not exist", cfg.Role),
				reason: "Could not use role",
			}
		}
		for _, msg := range roleSetup {
			content, err := loadMsg(msg)
			if err != nil {
				return modsError{
					err:    err,
					reason: "Could not use role",
				}
			}
			m.messages = append(m.messages, proto.Message{
				Role:    proto.RoleSystem,
				Content: content,
			})
		}
	}

	if !cfg.NoCache && cfg.cacheReadFromID != "" {
		if err := m.cache.Read(cfg.cacheReadFromID, &m.messages); err != nil {
			return modsError{
				err: err,
				reason: fmt.Sprintf(
					"There was a problem reading the cache. Use %s / %s to disable it.",
					m.Styles.InlineCode.Render("--no-cache"),
					m.Styles.InlineCode.Render("NO_CACHE"),
				),
			}
		}
	}

	if !m.HasOmittedAttachment() {
		if prefix := cfg.Prefix; prefix != "" {
			content = strings.TrimSpace(prefix + "\n\n" + content)
		}
		if !cfg.NoLimit && mod.MaxChars > 0 && int64(len(content)) > mod.MaxChars {
			content = content[:mod.MaxChars]
		}
		m.messages = append(m.messages, proto.Message{Role: proto.RoleUser, Content: content})
		return nil
	}

	parts := m.userInputParts(content)
	if prefix := cfg.Prefix; prefix != "" {
		parts = append([]proto.ContentPart{{Type: proto.ContentPartText, Text: prefix}}, parts...)
	}
	if !cfg.NoLimit && mod.MaxChars > 0 {
		parts = limitTextParts(parts, mod.MaxChars)
	}
	m.messages = append(m.messages, proto.Message{Role: proto.RoleUser, Content: textFromParts(parts), Parts: parts})

	return nil
}

func (m *Mods) userInputParts(content string) []proto.ContentPart {
	if content != m.rawInput {
		if strings.HasPrefix(m.rawInput, content) {
			return inputPartsForContent(m.inputParts, len(content))
		}
		// Retries produced by cutPrompt always preserve a prefix. Keep this
		// fallback defensive for any future retry source that does not.
		parts := make([]proto.ContentPart, 0, 2)
		if content != "" {
			parts = append(parts, proto.ContentPart{Type: proto.ContentPartText, Text: content})
		}
		for _, part := range m.inputParts {
			if part.Image != nil {
				parts = append(parts, part)
			}
		}
		return parts
	}
	return append([]proto.ContentPart(nil), m.inputParts...)
}

// inputPartsForContent keeps image positions while shortening the flattened
// text input. The flattened form joins text parts with blank lines, matching
// joinInputParts. Image data is never part of that text budget.
func inputPartsForContent(parts []proto.ContentPart, contentBytes int) []proto.ContentPart {
	result := make([]proto.ContentPart, 0, len(parts))
	remaining := contentBytes
	seenText := false
	for _, part := range parts {
		if part.Type != proto.ContentPartText {
			result = append(result, part)
			continue
		}
		if remaining <= 0 {
			continue
		}
		if seenText {
			const separator = "\n\n"
			if remaining <= len(separator) {
				for i := len(result) - 1; i >= 0; i-- {
					if result[i].Type == proto.ContentPartText {
						result[i].Text += separator[:remaining]
						break
					}
				}
				remaining = 0
				continue
			}
			remaining -= len(separator)
		}
		seenText = true
		if len(part.Text) > remaining {
			part.Text = part.Text[:remaining]
		}
		remaining -= len(part.Text)
		result = append(result, part)
	}
	return result
}

func limitTextParts(parts []proto.ContentPart, limit int64) []proto.ContentPart {
	result := make([]proto.ContentPart, 0, len(parts))
	remaining := limit
	for _, part := range parts {
		if part.Type != proto.ContentPartText {
			result = append(result, part)
			continue
		}
		if remaining <= 0 {
			continue
		}
		if int64(len(part.Text)) > remaining {
			part.Text = part.Text[:remaining]
		}
		remaining -= int64(len(part.Text))
		result = append(result, part)
	}
	return result
}

func textFromParts(parts []proto.ContentPart) string {
	text := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == proto.ContentPartText && part.Text != "" {
			text = append(text, part.Text)
		}
	}
	return strings.Join(text, "\n\n")
}
