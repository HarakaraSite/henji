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

func TestBuildModelsListOutput(t *testing.T) {
	cfg := &Config{
		API:   "openai",
		Model: "gpt-4o",
		APIs: APIs{
			{
				Name: "anthropic",
				Models: map[string]Model{
					"claude-sonnet-5": {Aliases: []string{"sonnet"}},
				},
			},
			{
				Name: "openai",
				Models: map[string]Model{
					"gpt-4o":      {Aliases: []string{"4o"}},
					"gpt-4o-mini": {},
				},
			},
		},
	}

	out := buildModelsListOutput(cfg)

	if out.Version != jsonSchemaVersion {
		t.Errorf("Version = %d, want %d", out.Version, jsonSchemaVersion)
	}
	if len(out.APIs) != 2 {
		t.Fatalf("len(APIs) = %d, want 2", len(out.APIs))
	}
	// sorted alphabetically: anthropic, openai
	if out.APIs[0].Name != "anthropic" || out.APIs[0].Default {
		t.Errorf("APIs[0] = %+v, want anthropic/non-default", out.APIs[0])
	}
	if out.APIs[1].Name != "openai" || !out.APIs[1].Default {
		t.Errorf("APIs[1] = %+v, want openai/default", out.APIs[1])
	}
	// sorted alphabetically: gpt-4o, gpt-4o-mini
	models := out.APIs[1].Models
	if len(models) != 2 || models[0].ID != "gpt-4o" || !models[0].Default {
		t.Errorf("Models[0] = %+v, want gpt-4o/default", models[0])
	}
	if models[1].ID != "gpt-4o-mini" || models[1].Default {
		t.Errorf("Models[1] = %+v, want gpt-4o-mini/non-default", models[1])
	}
}
