package main

import "testing"

func TestBuildJSONOutput(t *testing.T) {
	cfg := &Config{Model: "gpt-4"}
	cfg.cacheWriteToID = "abc123"
	mods := &Mods{Config: cfg, Output: "hello world"}

	out := buildJSONOutput(mods)

	if out.Version != jsonSchemaVersion {
		t.Errorf("Version = %d, want %d", out.Version, jsonSchemaVersion)
	}
	if out.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", out.Model, "gpt-4")
	}
	if out.ConversationID != "abc123" {
		t.Errorf("ConversationID = %q, want %q", out.ConversationID, "abc123")
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" || out.Content[0].Text != "hello world" {
		t.Errorf("Content = %+v, want single text block %q", out.Content, "hello world")
	}
}

func TestBuildJSONOutputEmpty(t *testing.T) {
	mods := &Mods{Config: &Config{}}

	out := buildJSONOutput(mods)

	if out.Content != nil {
		t.Errorf("Content = %+v, want nil for empty output", out.Content)
	}
	if out.ConversationID != "" {
		t.Errorf("ConversationID = %q, want empty", out.ConversationID)
	}
}
