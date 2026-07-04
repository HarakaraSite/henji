package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/template"
	"time"

	_ "embed"

	"github.com/caarlos0/env/v11"
	"github.com/muesli/termenv"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// configHomeDir returns the base directory henji's settings file lives
// under: $XDG_CONFIG_HOME if set, otherwise a single OS-specific default
// (no multi-location fallback search).
func configHomeDir() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return v, nil
		}
		return filepath.Join(home, "AppData", "Local"), nil
	}
	// macOS and Linux share the same default: ~/.config, matching how
	// most XDG-aware CLI tools (fish included) lay out their config.
	return filepath.Join(home, ".config"), nil
}

// dataHomeDir returns the base directory henji's cache (conversation
// history) lives under: $XDG_DATA_HOME if set, otherwise a single
// OS-specific default.
func dataHomeDir() (string, error) {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return v, nil
		}
		return filepath.Join(home, "AppData", "Local"), nil
	}
	// macOS and Linux share the same default: ~/.local/share, matching
	// how most XDG-aware CLI tools (fish included) lay out their data.
	return filepath.Join(home, ".local", "share"), nil
}

//go:embed config_template.yml
var configTemplate string

const (
	defaultMarkdownFormatText = "Format the response as markdown without enclosing backticks."
	defaultJSONFormatText     = "Format the response as json without enclosing backticks."
)

var help = map[string]string{
	"api":                   "OpenAI compatible REST API (openai, localai, anthropic, ...). default-model belongs to one API, so pair this with -m/--model or it will likely fail to find the model",
	"apis":                  "Aliases and endpoints for OpenAI compatible REST API",
	"http-proxy":            "HTTP proxy to use for API requests",
	"model":                 "Default model (gpt-3.5-turbo, gpt-4, ggml-gpt4all-j...)",
	"max-input-chars":       "Default character limit on input to model",
	"format":                "Ask for the response to be formatted as markdown unless otherwise set",
	"format-as":             "Format to request from the model when -f/--format is set; valid values are keys in format-text (default: markdown, json)",
	"format-text":           "Text to append when using the -f flag",
	"json-schema":           "Path to a JSON Schema file; the response is constrained to it (Anthropic/OpenAI-compatible/Google) and validated against it client-side. Google rejects some keywords (e.g. additionalProperties)",
	"json-schema-retries":   "Maximum number of times to ask the model to correct a response that failed JSON Schema validation",
	"role":                  "System role to use",
	"roles":                 "List of predefined system messages that can be used as roles",
	"list-roles":            "List the roles defined in your configuration file",
	"list-models":           "List configured APIs and their models (respects --output json)",
	"raw":                   "Render output as raw text when connected to a TTY",
	"quiet":                 "Quiet mode (hide the spinner while loading and stderr messages for success)",
	"help":                  "Show help and exit",
	"version":               "Show version and exit",
	"max-retries":           "Maximum number of times to retry API calls",
	"max-tool-calls":        "Maximum number of agentic tool call rounds (default: 0, unlimited). Tools only exist if mcp-servers are configured; check what's available with --mcp-list-tools before relying on this",
	"no-limit":              "Turn off the client-side limit on the size of the input into the model",
	"word-wrap":             "Wrap formatted output at specific width (default is 80)",
	"max-tokens":            "Maximum number of tokens in response",
	"max-completion-tokens": "Maximum number of completion tokens in response (overrides max-tokens; some reasoning models require this instead)",
	"temp":                  "Temperature (randomness) of results, from 0.0 to 2.0, -1.0 to disable",
	"stop":                  "Up to 4 sequences where the API will stop generating further tokens",
	"topp":                  "TopP, an alternative to temperature that narrows response, from 0.0 to 1.0, -1.0 to disable",
	"topk":                  "TopK, only sample from the top K options for each subsequent token, -1 to disable",
	"settings":              "Open settings in your $EDITOR",
	"continue":              "Continue from the last response or a given save title",
	"continue-last":         "Continue from the last response",
	"no-cache":              "Disables caching of the prompt/response",
	"title":                 "Saves the current conversation with the given title",
	"list":                  "Lists saved conversations",
	"delete":                "Deletes one or more saved conversations with the given titles or IDs",
	"show":                  "Show a saved conversation with the given title or ID",
	"editor":                "Edit the prompt in your $EDITOR; only taken into account if no other args and if STDIN is a TTY",
	"mcp-servers":           "MCP Servers configurations",
	"mcp-disable":           "Disable specific MCP servers",
	"mcp-list":              "List all available MCP servers",
	"mcp-list-tools":        "List all available tools from enabled MCP servers",
	"mcp-timeout":           "Timeout for MCP server calls, defaults to 15 seconds",
	"output":                `Output format: text or json. json prints one line: {"version":1,"content":[{"type":"text","text":"..."}],"model":"...","conversation_id":"..."} on success, {"version":1,"error":{"code":"...","message":"..."}} on failure (exit 1)`,
}

// Model represents the LLM model used in the API call.
type Model struct {
	Name                string
	API                 string
	MaxChars            int64    `yaml:"max-input-chars"`
	Aliases             []string `yaml:"aliases"`
	Fallback            string   `yaml:"fallback"`
	ThinkingBudget      int      `yaml:"thinking-budget,omitempty"`
	MaxCompletionTokens int64    `yaml:"max-completion-tokens,omitempty"`
}

// API represents an API endpoint and its models.
type API struct {
	Name      string
	APIKey    string           `yaml:"api-key"`
	APIKeyEnv string           `yaml:"api-key-env"`
	APIKeyCmd string           `yaml:"api-key-cmd"`
	Version   string           `yaml:"version"` // XXX: not used anywhere
	BaseURL   string           `yaml:"base-url"`
	Models    map[string]Model `yaml:"models"`
	User      string           `yaml:"user"`
}

// APIs is a type alias to allow custom YAML decoding.
type APIs []API

// UnmarshalYAML implements sorted API YAML decoding.
func (apis *APIs) UnmarshalYAML(node *yaml.Node) error {
	for i := 0; i < len(node.Content); i += 2 {
		var api API
		if err := node.Content[i+1].Decode(&api); err != nil {
			return fmt.Errorf("error decoding YAML file: %s", err)
		}
		api.Name = node.Content[i].Value
		*apis = append(*apis, api)
	}
	return nil
}

// FormatText is a map[format]formatting_text.
type FormatText map[string]string

// UnmarshalYAML conforms with yaml.Unmarshaler.
func (ft *FormatText) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err != nil {
		var formats map[string]string
		if err := unmarshal(&formats); err != nil {
			return err
		}
		*ft = (FormatText)(formats)
		return nil
	}

	*ft = map[string]string{
		"markdown": text,
	}
	return nil
}

// Config holds the main configuration and is mapped to the YAML settings file.
type Config struct {
	API                 string     `yaml:"default-api" env:"API"`
	Model               string     `yaml:"default-model" env:"MODEL"`
	Format              bool       `yaml:"format" env:"FORMAT"`
	FormatText          FormatText `yaml:"format-text"`
	FormatAs            string     `yaml:"format-as" env:"FORMAT_AS"`
	JSONSchemaPath      string     `yaml:"json-schema" env:"JSON_SCHEMA"`
	JSONSchemaRetries   int        `yaml:"json-schema-retries" env:"JSON_SCHEMA_RETRIES"`
	Raw                 bool       `yaml:"raw" env:"RAW"`
	Output              string     `yaml:"output" env:"OUTPUT"`
	Quiet               bool       `yaml:"quiet" env:"QUIET"`
	MaxTokens           int64      `yaml:"max-tokens" env:"MAX_TOKENS"`
	MaxCompletionTokens int64      `yaml:"max-completion-tokens" env:"MAX_COMPLETION_TOKENS"`
	MaxInputChars       int64      `yaml:"max-input-chars" env:"MAX_INPUT_CHARS"`
	Temperature         float64    `yaml:"temp" env:"TEMP"`
	Stop                []string   `yaml:"stop" env:"STOP"`
	TopP                float64    `yaml:"topp" env:"TOPP"`
	TopK                int64      `yaml:"topk" env:"TOPK"`
	NoLimit             bool       `yaml:"no-limit" env:"NO_LIMIT"`
	CachePath           string     `yaml:"cache-path" env:"CACHE_PATH"`
	NoCache             bool       `yaml:"no-cache" env:"NO_CACHE"`
	MaxRetries          int        `yaml:"max-retries" env:"MAX_RETRIES"`
	MaxToolCalls        int        `yaml:"max-tool-calls" env:"MAX_TOOL_CALLS"`
	WordWrap            int        `yaml:"word-wrap" env:"WORD_WRAP"`
	HTTPProxy           string     `yaml:"http-proxy" env:"HTTP_PROXY"`
	APIs                APIs       `yaml:"apis"`
	Role                string     `yaml:"role" env:"ROLE"`
	Roles               map[string][]string
	ShowHelp            bool
	Prefix              string
	Version             bool
	Settings            bool
	SettingsPath        string
	ContinueLast        bool
	Continue            string
	Title               string
	Show                string
	List                bool
	ListRoles           bool
	ListModels          bool
	Delete              []string
	User                string

	MCPServers   map[string]MCPServerConfig `yaml:"mcp-servers"`
	MCPList      bool
	MCPListTools bool
	MCPDisable   []string
	MCPTimeout   time.Duration `yaml:"mcp-timeout" env:"MCP_TIMEOUT"`
	// Note: the former top-level "system:" YAML key (Config.System) has been
	// removed. Use the "roles:" section to define system prompts instead.

	openEditor                                         bool
	cacheReadFromID, cacheWriteToID, cacheWriteToTitle string

	// jsonSchemaDoc is the parsed --json-schema file, sent to providers as-is.
	// jsonSchemaValidator compiles the same file for client-side validation.
	// Both are populated once in RunE if JSONSchemaPath is set.
	jsonSchemaDoc       map[string]any
	jsonSchemaValidator *jsonschema.Schema
}

// MCPServerConfig holds configuration for an MCP server.
type MCPServerConfig struct {
	Type    string   `yaml:"type"`
	Command string   `yaml:"command"`
	Env     []string `yaml:"env"`
	Args    []string `yaml:"args"`
	URL     string   `yaml:"url"`
}

func ensureConfig() (Config, error) {
	// Start from defaults so an omitted setting remains distinguishable from
	// an explicitly configured zero value. This matters for sampling options:
	// zero is valid (for example, temp: 0), while -1 means "do not send".
	c := defaultConfig()
	configHome, err := configHomeDir()
	if err != nil {
		return c, modsError{err, "Could not find settings path."}
	}
	sp := filepath.Join(configHome, "henji", "henji.yml")
	c.SettingsPath = sp

	dir := filepath.Dir(sp)
	if dirErr := os.MkdirAll(dir, 0o700); dirErr != nil { //nolint:mnd
		return c, modsError{dirErr, "Could not create cache directory."}
	}

	if dirErr := writeConfigFile(sp); dirErr != nil {
		return c, dirErr
	}
	content, err := os.ReadFile(sp)
	if err != nil {
		return c, modsError{err, "Could not read settings file."}
	}
	if err := yaml.Unmarshal(content, &c); err != nil {
		return c, modsError{err, "Could not parse settings file."}
	}

	if err := env.ParseWithOptions(&c, env.Options{Prefix: "HENJI_"}); err != nil {
		return c, modsError{err, "Could not parse environment into settings file."}
	}

	if c.CachePath == "" {
		dataHome, err := dataHomeDir()
		if err != nil {
			return c, modsError{err, "Could not find cache path."}
		}
		c.CachePath = filepath.Join(dataHome, "henji")
	}

	if err := os.MkdirAll(
		filepath.Join(c.CachePath, "conversations"),
		0o700,
	); err != nil { //nolint:mnd
		return c, modsError{err, "Could not create cache directory."}
	}

	if c.WordWrap == 0 {
		c.WordWrap = 80
	}

	return c, nil
}

func writeConfigFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return createConfigFile(path)
	} else if err != nil {
		return modsError{err, "Could not stat path."}
	}
	return nil
}

func createConfigFile(path string) error {
	tmpl := template.Must(template.New("config").Parse(configTemplate))

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return modsError{err, "Could not create configuration file."}
	}
	defer func() { _ = f.Close() }()

	m := struct {
		Config Config
		Help   map[string]string
	}{
		Config: defaultConfig(),
		Help:   help,
	}
	if err := tmpl.Execute(f, m); err != nil {
		return modsError{err, "Could not render template."}
	}
	return nil
}

func defaultConfig() Config {
	return Config{
		Output:      "text",
		FormatAs:    "markdown",
		Temperature: -1,
		TopP:        -1,
		TopK:        -1,
		FormatText: FormatText{
			"markdown": defaultMarkdownFormatText,
			"json":     defaultJSONFormatText,
		},
		MCPTimeout:        15 * time.Second,
		JSONSchemaRetries: 2,
	}
}

func useLine() string {
	appName := filepath.Base(os.Args[0])

	if stdoutRenderer().ColorProfile() == termenv.TrueColor {
		appName = makeGradientText(stdoutStyles().AppName, appName)
	}

	return fmt.Sprintf(
		"%s %s",
		appName,
		stdoutStyles().CliArgs.Render("[OPTIONS] [PREFIX TERM]"),
	)
}

func usageFunc(cmd *cobra.Command) error {
	if cmd != rootCmd {
		fmt.Printf("Usage:\n  %s\n", cmd.UseLine())
		if cmd.HasAvailableLocalFlags() {
			fmt.Println("\nOptions:")
			printFlags(cmd)
		}
		return nil
	}

	fmt.Printf(
		"Usage:\n  %s\n\n",
		useLine(),
	)
	commands := cmd.Commands()
	visibleCommands := make([]*cobra.Command, 0, len(commands))
	for _, sub := range commands {
		if !sub.Hidden && sub.IsAvailableCommand() {
			visibleCommands = append(visibleCommands, sub)
		}
	}
	if len(visibleCommands) > 0 {
		fmt.Println("Commands:")
		for _, sub := range visibleCommands {
			fmt.Printf("  %-44s %s\n", sub.Name(), sub.Short)
		}
		fmt.Println()
	}
	fmt.Println("Options:")
	printFlags(cmd)
	if len(helpExamples) > 0 {
		fmt.Println("\nExamples:")
		for _, ex := range helpExamples {
			fmt.Printf(
				"  %s\n  %s\n\n",
				stdoutStyles().Comment.Render("# "+ex.title),
				cheapHighlighting(stdoutStyles(), ex.command),
			)
		}
	}
	fmt.Printf("Full manual:\n  %s\n", cheapHighlighting(stdoutStyles(), "henji docs"))

	return nil
}

func printFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *flag.Flag) {
		if f.Hidden {
			return
		}
		if f.Shorthand == "" {
			fmt.Printf(
				"  %-44s %s\n",
				stdoutStyles().Flag.Render("--"+f.Name),
				stdoutStyles().FlagDesc.Render(f.Usage),
			)
		} else {
			fmt.Printf(
				"  %s%s %-40s %s\n",
				stdoutStyles().Flag.Render("-"+f.Shorthand),
				stdoutStyles().FlagComma,
				stdoutStyles().Flag.Render("--"+f.Name),
				stdoutStyles().FlagDesc.Render(f.Usage),
			)
		}
	})
}
