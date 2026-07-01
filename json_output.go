package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// jsonSchemaVersion is the version of the --output json envelope. Bump this
// whenever a field is renamed or removed (adding an omitempty field does not
// require a bump).
const jsonSchemaVersion = 1

// JSONOutput is the envelope printed to stdout when --output json is set.
type JSONOutput struct {
	Version        int            `json:"version"`
	ConversationID string         `json:"conversation_id,omitempty"`
	Content        []ContentBlock `json:"content,omitempty"`
	Model          string         `json:"model,omitempty"`
	Error          *ErrorInfo     `json:"error,omitempty"`
}

// ContentBlock is a typed piece of response content. Only "text" is produced
// today; "tool_call"/"tool_result"/"artifact" etc. can be added later without
// breaking consumers that only look at type:"text" blocks.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ErrorInfo describes a failure in --output json mode.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func buildJSONOutput(mods *Mods) JSONOutput {
	out := JSONOutput{
		Version: jsonSchemaVersion,
		Model:   mods.Config.Model,
	}
	if mods.Config.cacheWriteToID != "" {
		out.ConversationID = mods.Config.cacheWriteToID
	}
	if mods.Output != "" {
		out.Content = []ContentBlock{{Type: "text", Text: mods.Output}}
	}
	return out
}

func printJSONOutput(mods *Mods) {
	out, err := json.Marshal(buildJSONOutput(mods))
	if err != nil {
		// should never happen: JSONOutput only contains marshalable fields.
		fmt.Fprintln(os.Stderr, "could not marshal --output json response:", err)
		return
	}
	fmt.Println(string(out))
}

func printJSONError(mods *Mods, err modsError) {
	out := buildJSONOutput(mods)
	out.Error = &ErrorInfo{
		Code:    "error",
		Message: fmt.Sprintf("%s: %s", err.Reason(), err.Error()),
	}
	marshaled, marshalErr := json.Marshal(out)
	if marshalErr != nil {
		fmt.Fprintln(os.Stderr, "could not marshal --output json error:", marshalErr)
		return
	}
	fmt.Println(string(marshaled))
}

// ModelsListOutput is the envelope printed by --list-models --output json.
type ModelsListOutput struct {
	Version int              `json:"version"`
	APIs    []APIModelsEntry `json:"apis"`
}

// APIModelsEntry describes one configured API endpoint and its models.
type APIModelsEntry struct {
	Name    string       `json:"name"`
	Default bool         `json:"default,omitempty"`
	Models  []ModelEntry `json:"models"`
}

// ModelEntry describes one model available under an API endpoint.
type ModelEntry struct {
	ID      string   `json:"id"`
	Aliases []string `json:"aliases,omitempty"`
	Default bool     `json:"default,omitempty"`
}

func buildModelsListOutput(cfg *Config) ModelsListOutput {
	out := ModelsListOutput{Version: jsonSchemaVersion}
	for _, api := range cfg.APIs {
		entry := APIModelsEntry{
			Name:    api.Name,
			Default: api.Name == cfg.API,
		}
		for name, mod := range api.Models {
			entry.Models = append(entry.Models, ModelEntry{
				ID:      name,
				Aliases: mod.Aliases,
				Default: entry.Default && name == cfg.Model,
			})
		}
		sort.Slice(entry.Models, func(i, j int) bool { return entry.Models[i].ID < entry.Models[j].ID })
		out.APIs = append(out.APIs, entry)
	}
	sort.Slice(out.APIs, func(i, j int) bool { return out.APIs[i].Name < out.APIs[j].Name })
	return out
}

func printModelsListJSON(cfg *Config) {
	out, err := json.Marshal(buildModelsListOutput(cfg))
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not marshal --list-models json:", err)
		return
	}
	fmt.Println(string(out))
}
