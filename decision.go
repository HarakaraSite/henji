package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"forge.harakara.site/littleisland/henji/v2/internal/decision"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

func newDecisionCmd() *cobra.Command {
	var apiName, modelName, questionsPath, imagePath string
	cmd := &cobra.Command{
		Use:   "decision -m MODEL --questions FILE [-a API] [--image FILE]",
		Short: "Evaluate text or an image with a Decisions API",
		Long: "Read evidence from stdin and questions from a provider-native JSON file.\n" +
			"Print the complete native response JSON; no conversation is saved.\n" +
			"Normal refusals exit 0. Failures leave stdout empty and exit 1.",
		Example:      "  henji decision -a openrouter -m typesafe/jev-1.13 --questions questions.json < input.txt",
		SilenceUsage: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return fmt.Errorf("the decision subcommand reads evidence from stdin; to use decision in a generation prompt, quote it, e.g.: henji \"decision %s\"", strings.Join(args, " "))
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDecision(cmd, apiName, modelName, questionsPath, imagePath)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&apiName, "api", "a", "", "API entry (default: configured default-api)")
	flags.StringVarP(&modelName, "model", "m", "", "Decision model ID or configured alias (required)")
	flags.Var(&singlePathValue{path: &questionsPath, name: "questions"}, "questions", "Provider-native questions JSON file (required)")
	flags.Var(&singlePathValue{path: &imagePath, name: "image"}, "image", "One JPEG, PNG, or WebP image (max 3 MiB; vision: true)")
	_ = cmd.MarkFlagRequired("model")
	_ = cmd.MarkFlagRequired("questions")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w (see henji decision --help)", err)
	})
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Usage:\n  %s\n\nOptions:\n%s\nFull manual: henji docs\n", cmd.UseLine(), cmd.LocalFlags().FlagUsages())
		return err
	})
	return cmd
}

// isDecisionCmd follows Cobra's command discovery, including flag values and
// the -- separator. Completion of decision also needs no conversation storage.
// main registers root flags before calling this on configuration error paths.
func isDecisionCmd(args []string) bool {
	if len(args) < 2 {
		return false
	}
	args = args[1:]
	if args[0] == "__complete" || args[0] == "__completeNoDesc" {
		args = args[1:]
	}
	cmd, _, _ := rootCmd.Find(args)
	return cmd != nil && cmd.Name() == "decision" && cmd.Parent() == rootCmd
}

func resolveDecisionAPI(cfg *Config, apiName, modelName string) (API, Model, error) {
	if apiName == "" {
		apiName = cfg.API
	}
	if apiName == "" {
		return API{}, Model{}, newUserErrorf("specify --api or configure default-api for henji decision")
	}
	if strings.TrimSpace(modelName) == "" {
		return API{}, Model{}, newUserErrorf("henji decision requires an explicit --model")
	}
	api := API{Name: apiName}
	found := false
	for _, configured := range cfg.APIs {
		if configured.Name == apiName {
			api, found = configured, true
			break
		}
	}
	if !found && apiName != "openai" && apiName != "openrouter" {
		return API{}, Model{}, newUserErrorf("API %q is not configured in %s", apiName, settingsPathHint(cfg))
	}
	if api.DecisionProtocol == "" && (apiName == "openai" || apiName == "openrouter") {
		api.DecisionProtocol = apiName
	}
	switch api.DecisionProtocol {
	case "openai":
		if api.DecisionBaseURL == "" {
			api.DecisionBaseURL = "https://api.openai.com/v1"
		}
	case "openrouter":
		if api.DecisionBaseURL == "" {
			api.DecisionBaseURL = "https://openrouter.ai/api/alpha"
		}
	default:
		return API{}, Model{}, newUserErrorf("API %q requires decision-protocol: openai or openrouter", apiName)
	}
	// Model IDs need no catalog entry. Configured aliases and vision capability
	// remain useful, while newly available decision models can be selected as-is.
	model := Model{Name: modelName, API: apiName}
	if configured, ok := api.Models[modelName]; ok {
		model = configured
		model.Name, model.API = modelName, apiName
	} else {
		for name, configured := range api.Models {
			if slices.Contains(configured.Aliases, modelName) {
				model = configured
				model.Name, model.API = name, apiName
				break
			}
		}
	}
	return api, model, nil
}

func runDecision(cmd *cobra.Command, apiName, modelName, questionsPath, imagePath string) error {
	api, model, err := resolveDecisionAPI(&config, apiName, modelName)
	if err != nil {
		return err
	}
	if config.MaxRetries < 0 {
		return newUserErrorf("max-retries must be non-negative for henji decision")
	}
	questions, err := os.ReadFile(questionsPath)
	if err != nil {
		return fmt.Errorf("could not read --questions file: %w", err)
	}
	parsedQuestions, err := decision.ParseQuestions(api.DecisionProtocol, questions)
	if err != nil {
		return fmt.Errorf("invalid --questions file: %w", err)
	}
	image, err := readImageInput(imagePath)
	if err != nil {
		return err
	}
	if image != nil && !model.Vision {
		return newUserErrorf("model %q requires vision: true in %s to use --image", model.Name, settingsPathHint(&config))
	}
	var input []byte
	reader := cmd.InOrStdin()
	if reader != os.Stdin || !isatty.IsTerminal(os.Stdin.Fd()) {
		closer, ok := reader.(io.ReadCloser)
		if !ok {
			closer = io.NopCloser(reader)
		}
		input, err = readAllContext(cmd.Context(), closer)
		if err != nil {
			return fmt.Errorf("could not read decision input: %w", err)
		}
	}
	if !utf8.Valid(input) {
		return newUserErrorf("decision input is not valid UTF-8")
	}
	if strings.TrimSpace(string(input)) == "" && image == nil {
		return newUserErrorf("henji decision requires text on stdin and/or --image")
	}
	defaultEnv, keyURL := "OPENAI_API_KEY", "https://platform.openai.com/api-keys"
	if api.DecisionProtocol == "openrouter" {
		defaultEnv, keyURL = "OPENROUTER_API_KEY", "https://openrouter.ai/settings/keys"
	}
	credentials := Mods{Config: &config, Styles: stderrStyles()}
	key, err := credentials.ensureKeyContext(cmd.Context(), api, defaultEnv, keyURL)
	if err != nil {
		return err
	}
	client := &http.Client{}
	if config.HTTPProxy != "" {
		proxyURL, err := url.Parse(config.HTTPProxy)
		if err != nil {
			return fmt.Errorf("invalid http-proxy: %w", err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyURL(proxyURL)
		client.Transport = transport
		defer transport.CloseIdleConnections()
	}
	response, err := decision.Execute(cmd.Context(), decision.Config{
		Protocol: api.DecisionProtocol, BaseURL: api.DecisionBaseURL,
		APIKey: key, HTTPClient: client, MaxRetries: config.MaxRetries,
	}, decision.Request{Model: model.Name, Text: string(input), Questions: parsedQuestions, Image: image})
	if err != nil {
		return fmt.Errorf("decision request failed: %w", err)
	}
	// Write only after a complete, valid JSON response. Keep the provider's bytes
	// and unknown fields rather than marshaling a normalized response structure.
	if len(response) == 0 || response[len(response)-1] != '\n' {
		response = append(response, '\n')
	}
	_, err = cmd.OutOrStdout().Write(response)
	return err
}
