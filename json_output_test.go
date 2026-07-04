package main

import (
	"encoding/json"
	"strings"
	"testing"
)

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
				Name:    "openai",
				BaseURL: "https://api.openai.com/v1",
				Models: map[string]Model{
					"gpt-4o":      {Aliases: []string{"4o"}},
					"gpt-4o-mini": {},
				},
			},
			{
				Name:    "localai",
				BaseURL: "http://localhost:8080/v1",
				Models: map[string]Model{
					"local-model": {},
				},
			},
		},
	}

	out := buildModelsListOutput(cfg)

	if out.Version != jsonSchemaVersion {
		t.Errorf("Version = %d, want %d", out.Version, jsonSchemaVersion)
	}
	if len(out.APIs) != 3 {
		t.Fatalf("len(APIs) = %d, want 3", len(out.APIs))
	}
	// sorted alphabetically: anthropic, localai, openai
	if out.APIs[0].Name != "anthropic" || out.APIs[0].Default || out.APIs[0].BaseURL != "" {
		t.Errorf("APIs[0] = %+v, want anthropic/non-default/no base_url", out.APIs[0])
	}
	if out.APIs[1].Name != "localai" || out.APIs[1].BaseURL != "http://localhost:8080/v1" {
		t.Errorf("APIs[1] = %+v, want localai with base_url", out.APIs[1])
	}
	if out.APIs[2].Name != "openai" || !out.APIs[2].Default || out.APIs[2].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("APIs[2] = %+v, want openai/default with base_url", out.APIs[2])
	}
	// sorted alphabetically: gpt-4o, gpt-4o-mini
	models := out.APIs[2].Models
	if len(models) != 2 || models[0].ID != "gpt-4o" || !models[0].Default {
		t.Errorf("Models[0] = %+v, want gpt-4o/default", models[0])
	}
	if models[1].ID != "gpt-4o-mini" || models[1].Default {
		t.Errorf("Models[1] = %+v, want gpt-4o-mini/non-default", models[1])
	}
}

// TestBuildModelsListOutputBaseURLOmittedWhenUnset is a regression test for
// backlog item 5 (model-eval-results 2026-07): base_url lets scripts tell
// local gateways apart from cloud endpoints. It must be omitted (not an
// empty string) when an API entry doesn't set base-url, since json:",omitempty"
// is what keeps this an additive, non-breaking change to the envelope.
func TestBuildModelsListOutputBaseURLOmittedWhenUnset(t *testing.T) {
	cfg := &Config{
		APIs: APIs{
			{Name: "anthropic", Models: map[string]Model{"claude-sonnet-5": {}}},
		},
	}

	out, err := json.Marshal(buildModelsListOutput(cfg))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(out), "base_url") {
		t.Errorf("output contains base_url when unset: %s", out)
	}
}
