package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"forge.harakara.site/littleisland/henji/v2/internal/anthropic"
	"forge.harakara.site/littleisland/henji/v2/internal/cache"
	"forge.harakara.site/littleisland/henji/v2/internal/google"
	"forge.harakara.site/littleisland/henji/v2/internal/openai"
	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"forge.harakara.site/littleisland/henji/v2/internal/stream"
	"github.com/caarlos0/go-shellwords"
	"github.com/charmbracelet/x/exp/ordered"
)

// Mods executes one henji request and retains the response for output and
// conversation caching. It deliberately has no terminal event loop.
type Mods struct {
	Output        string
	Input         string
	Styles        styles
	retries       int
	schemaRetries int
	messages      []proto.Message
	inputParts    []proto.ContentPart
	rawInput      string

	db     *convoDB
	cache  *cache.Conversations
	Config *Config
	ctx    context.Context
}

func newMods(ctx context.Context, cfg *Config, db *convoDB, cache *cache.Conversations) *Mods {
	return &Mods{
		Styles: stderrStyles(),
		db:     db,
		cache:  cache,
		Config: cfg,
		ctx:    ctx,
	}
}

// run prepares conversation state, reads piped input, and completes a model
// request. Commands that only inspect local state are handled by main.go.
func (m *Mods) run() error {
	details, err := m.findCacheOpsDetails()
	if err != nil {
		return err
	}
	m.Config.cacheWriteToID = details.WriteID
	m.Config.cacheWriteToTitle = details.Title
	m.Config.cacheReadFromID = details.ReadID
	m.Config.API = details.API
	m.Config.Model = details.Model

	textInput, err := readTextInput(m.Config.textPath)
	if err != nil {
		return err
	}
	imageInput, err := readImageInput(m.Config.imagePath)
	if err != nil {
		return err
	}
	stdinInput, err := m.readStdin()
	if err != nil {
		return err
	}
	input := joinInputParts(textInput, stdinInput)
	m.Input = removeWhitespace(input)
	m.rawInput = input
	m.inputParts = buildInputParts(textInput, imageInput, stdinInput)

	if m.Config.Show != "" {
		return m.readFromCache()
	}
	if m.Input == "" && !m.HasImage() && m.Config.Prefix == "" {
		return nil
	}
	return m.complete(input)
}

// HasImage reports whether the current request has an image attachment.
func (m *Mods) HasImage() bool {
	for _, part := range m.inputParts {
		if part.Image != nil {
			return true
		}
	}
	return false
}

type completionStarter func(string) (stream.Stream, func(string) stream.Stream, Model, error)

func (m *Mods) complete(content string) error {
	return m.completeWith(content, m.startCompletion)
}

// completeWith keeps the retry loop independent from provider construction so
// the full fallback path can be tested with deterministic streams.
func (m *Mods) completeWith(content string, start completionStarter) error {
	var spinner *progressSpinner
	defer func() { spinner.stop() }()

	for {
		// Google starts the HTTP request synchronously inside Client.Request,
		// which is reached by startCompletion. Start progress reporting before
		// that call so connection and first-byte latency remain visible.
		if spinner == nil {
			spinner = startSpinner(os.Stderr, m.Config.Quiet)
		}
		response, retrySchema, mod, err := start(content)
		if err != nil {
			return err
		}
		err = m.consumeCompletion(response, retrySchema)
		if err == nil {
			return nil
		}

		err = m.handleRequestError(err, mod, content)
		var retry retryRequest
		if !errors.As(err, &retry) {
			return err
		}
		content = retry.content
	}
}

func (m *Mods) startCompletion(content string) (stream.Stream, func(string) stream.Stream, Model, error) {
	var ccfg openai.Config
	var accfg anthropic.Config
	var gccfg google.Config

	cfg := m.Config
	api, mod, err := m.resolveModel(cfg)
	cfg.API = mod.API
	if err != nil {
		return nil, nil, Model{}, err
	}
	if api.Name == "" {
		eps := make([]string, 0, len(cfg.APIs))
		for _, a := range cfg.APIs {
			eps = append(eps, m.Styles.InlineCode.Render(a.Name))
		}
		return nil, nil, Model{}, modsError{
			err:    newUserErrorf("Your configured API endpoints are: %s", eps),
			reason: fmt.Sprintf("The API endpoint %s is not configured.", m.Styles.InlineCode.Render(cfg.API)),
		}
	}
	if m.HasImage() && !mod.Vision {
		return nil, nil, Model{}, modsError{
			err:    newUserErrorf("model %q is not configured for vision input", mod.Name),
			reason: "Image input requires a model with vision: true in henji.yml.",
		}
	}

	switch mod.API {
	case "anthropic":
		key, err := m.ensureKey(api, "ANTHROPIC_API_KEY", "https://console.anthropic.com/settings/keys")
		if err != nil {
			return nil, nil, Model{}, modsError{err, "Anthropic authentication failed"}
		}
		accfg = anthropic.DefaultConfig(key)
		if api.BaseURL != "" {
			accfg.BaseURL = api.BaseURL
		}
	case "google":
		key, err := m.ensureKey(api, "GOOGLE_API_KEY", "https://aistudio.google.com/app/apikey")
		if err != nil {
			return nil, nil, Model{}, modsError{err, "Google authentication failed"}
		}
		gccfg = google.DefaultConfig(mod.Name, key)
		gccfg.ThinkingBudget = mod.ThinkingBudget
	case "azure", "azure-ad":
		key, err := m.ensureKey(api, "AZURE_OPENAI_KEY", "https://aka.ms/oai/access")
		if err != nil {
			return nil, nil, Model{}, modsError{err, "Azure authentication failed"}
		}
		ccfg = openai.Config{AuthToken: key, BaseURL: api.BaseURL}
		if mod.API == "azure-ad" {
			ccfg.APIType = "azure-ad"
		}
		if api.User != "" {
			cfg.User = api.User
		}
	default:
		key, err := m.ensureKey(api, "OPENAI_API_KEY", "https://platform.openai.com/account/api-keys")
		if err != nil {
			return nil, nil, Model{}, modsError{err, "OpenAI authentication failed"}
		}
		ccfg = openai.Config{AuthToken: key, BaseURL: api.BaseURL}
	}

	if cfg.HTTPProxy != "" {
		proxyURL, err := url.Parse(cfg.HTTPProxy)
		if err != nil {
			return nil, nil, Model{}, modsError{err, "There was an error parsing your proxy URL."}
		}
		httpClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
		ccfg.HTTPClient = httpClient
		accfg.HTTPClient = httpClient
		gccfg.HTTPClient = httpClient
	}

	if mod.MaxChars == 0 {
		mod.MaxChars = cfg.MaxInputChars
	}
	if mod.MaxCompletionTokens > 0 {
		cfg.MaxCompletionTokens = mod.MaxCompletionTokens
	}
	if strings.HasPrefix(mod.Name, "o1") {
		if cfg.MaxCompletionTokens == 0 && cfg.MaxTokens > 0 {
			cfg.MaxCompletionTokens = cfg.MaxTokens
		}
		cfg.MaxTokens = 0
	}
	if err := m.setupStreamContext(content, mod); err != nil {
		return nil, nil, Model{}, err
	}

	request := proto.Request{
		Messages:    m.messages,
		API:         mod.API,
		Model:       mod.Name,
		User:        cfg.User,
		Temperature: ptrOrNil(cfg.Temperature),
		TopP:        ptrOrNil(cfg.TopP),
		TopK:        ptrOrNil(cfg.TopK),
		Stop:        cfg.Stop,
		JSONSchema:  cfg.jsonSchemaDoc,
	}
	if cfg.MaxTokens > 0 {
		request.MaxTokens = &cfg.MaxTokens
	}
	if cfg.MaxCompletionTokens > 0 {
		request.MaxCompletionTokens = &cfg.MaxCompletionTokens
	}

	var client stream.Client
	switch mod.API {
	case "anthropic":
		client = anthropic.New(accfg)
	case "google":
		client = google.New(gccfg)
	default:
		client = openai.New(ccfg)
		if cfg.Format && cfg.FormatAs == "json" {
			request.ResponseFormat = &cfg.FormatAs
		}
	}

	retrySchema := func(correction string) stream.Stream {
		retryRequest := request
		retryRequest.Messages = append(append([]proto.Message{}, m.messages...), proto.Message{
			Role: proto.RoleUser, Content: correction,
		})
		return client.Request(m.ctx, retryRequest)
	}
	return client.Request(m.ctx, request), retrySchema, mod, nil
}

func (m *Mods) consumeCompletion(response stream.Stream, retrySchema func(string) stream.Stream) error {
	for {
		for response.Next() {
			chunk, err := response.Current()
			if err != nil && !errors.Is(err, stream.ErrNoContent) {
				_ = response.Close()
				return err
			}
			m.appendToOutput(chunk.Content)
		}
		if err := response.Err(); err != nil {
			return err
		}

		m.messages = response.Messages()
		correction, err := m.checkJSONSchema()
		if err != nil {
			return err
		}
		if correction == "" {
			return nil
		}
		response = retrySchema(correction)
	}
}

// checkJSONSchema returns a correction prompt when another attempt is needed.
func (m *Mods) checkJSONSchema() (string, error) {
	if m.Config.jsonSchemaValidator == nil {
		return "", nil
	}
	var content string
	if len(m.messages) > 0 {
		content = m.messages[len(m.messages)-1].Content
	}
	if err := validateAgainstSchema(m.Config.jsonSchemaValidator, content); err == nil {
		return "", nil
	} else {
		m.schemaRetries++
		if m.schemaRetries > m.Config.JSONSchemaRetries {
			return "", modsError{err: err, reason: fmt.Sprintf("Response did not match --json-schema after %d attempt(s).", m.schemaRetries)}
		}
		m.Output = ""
		return fmt.Sprintf("Your previous response did not conform to the required JSON Schema: %s\nRespond again with ONLY JSON that strictly matches the schema, and nothing else.", err), nil
	}
}

func (m *Mods) findCacheOpsDetails() (cacheDetailsMsg, error) {
	continueLast := m.Config.ContinueLast || (m.Config.Continue != "" && m.Config.Title == "")
	readID := ordered.First(m.Config.Continue, m.Config.Show)
	writeID := ordered.First(m.Config.Title, m.Config.Continue)
	title := writeID
	model := m.Config.Model
	api := m.Config.API

	if readID != "" || continueLast {
		found, err := m.findReadID(readID)
		if err != nil {
			return cacheDetailsMsg{}, modsError{err: err, reason: "Could not find the conversation."}
		}
		if found != nil {
			readID = found.ID
			if found.Model != nil && found.API != nil {
				model = *found.Model
				api = *found.API
			}
		}
	}
	if continueLast {
		writeID = readID
	}
	if writeID == "" {
		writeID = newConversationID()
	}
	if !sha1reg.MatchString(writeID) {
		convo, err := m.db.Find(writeID)
		if err != nil {
			writeID = newConversationID()
		} else {
			writeID = convo.ID
		}
	}
	return cacheDetailsMsg{WriteID: writeID, Title: title, ReadID: readID, API: api, Model: model}, nil
}

type cacheDetailsMsg struct{ WriteID, Title, ReadID, API, Model string }

func (m *Mods) findReadID(in string) (*Conversation, error) {
	convo, err := m.db.Find(in)
	if err == nil {
		return convo, nil
	}
	if errors.Is(err, errNoMatches) && m.Config.Show == "" {
		return m.db.FindHEAD()
	}
	return nil, err
}

func (m *Mods) readStdin() (string, error) {
	if isInputTTY() {
		return "", nil
	}
	stdinBytes, err := readAllContext(m.ctx, os.Stdin)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", modsError{err, "Interrupted while reading stdin."}
		}
		return "", modsError{err, "Unable to read stdin."}
	}
	return increaseIndent(string(stdinBytes)), nil
}

const maxAttachmentBytes = 3 * 1024 * 1024

func readTextInput(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", modsError{err, "Could not read text input."}
	}
	if info.Size() > maxAttachmentBytes {
		return "", attachmentTooLargeError(path, info.Size())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", modsError{err, "Could not read text input."}
	}
	if len(content) > maxAttachmentBytes {
		return "", attachmentTooLargeError(path, int64(len(content)))
	}
	if strings.IndexByte(string(content), 0) >= 0 || !utf8.Valid(content) {
		return "", modsError{
			err:    fmt.Errorf("%q is not valid UTF-8 text", path),
			reason: "Text input appears to be binary; --text only accepts UTF-8 text.",
		}
	}
	return string(content), nil
}

func attachmentTooLargeError(path string, size int64) modsError {
	return modsError{
		err:    fmt.Errorf("%q is %d bytes", path, size),
		reason: "Attachment exceeds the 3 MiB limit.",
	}
}

func readImageInput(path string) (*proto.Image, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, modsError{err, "Could not read image input."}
	}
	if info.Size() > maxAttachmentBytes {
		return nil, attachmentTooLargeError(path, info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, modsError{err, "Could not read image input."}
	}
	if len(data) > maxAttachmentBytes {
		return nil, attachmentTooLargeError(path, int64(len(data)))
	}
	mediaType := imageMediaType(data)
	if mediaType == "" {
		return nil, modsError{
			err:    fmt.Errorf("%q is not a supported image format", path),
			reason: "--image accepts JPEG, PNG, or WebP files.",
		}
	}
	return &proto.Image{MediaType: mediaType, Data: data}, nil
}

func imageMediaType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png"
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return ""
	}
}

func buildInputParts(text string, image *proto.Image, stdin string) []proto.ContentPart {
	parts := make([]proto.ContentPart, 0, 3)
	if removeWhitespace(text) != "" {
		parts = append(parts, proto.ContentPart{Type: proto.ContentPartText, Text: text})
	}
	if image != nil {
		parts = append(parts, proto.ContentPart{Type: proto.ContentPartImage, Image: image})
	}
	if removeWhitespace(stdin) != "" {
		parts = append(parts, proto.ContentPart{Type: proto.ContentPartText, Text: stdin})
	}
	return parts
}

// joinInputParts preserves the deterministic input order documented for
// --text: attached text content precedes stdin content, and both follow the
// instruction passed as command arguments in setupStreamContext.
func joinInputParts(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if removeWhitespace(part) != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}

// readAllContext waits for EOF without making an interrupted pipeline
// unkillable. Closing the reader unblocks io.ReadAll when Ctrl-C cancels the
// root command context.
func readAllContext(ctx context.Context, reader io.ReadCloser) ([]byte, error) {
	type result struct {
		content []byte
		err     error
	}
	results := make(chan result, 1)
	go func() {
		content, err := io.ReadAll(reader)
		results <- result{content: content, err: err}
	}()

	select {
	case result := <-results:
		return result.content, result.err
	case <-ctx.Done():
		_ = reader.Close()
		return nil, ctx.Err()
	}
}

func (m *Mods) readFromCache() error {
	var messages []proto.Message
	if err := m.cache.Read(m.Config.cacheReadFromID, &messages); err != nil {
		return modsError{err, "There was an error loading the conversation."}
	}
	m.messages = messages
	m.appendToOutput(proto.Conversation(messages).String())
	return nil
}

func (m *Mods) appendToOutput(s string) {
	m.Output += s
	if m.Config.Output == "json" || m.Config.jsonSchemaValidator != nil {
		return
	}
	if !isOutputTTY() || m.Config.Raw {
		fmt.Print(s)
	}
}

// printTextOutput finishes text-mode output after a request completes.
func (m *Mods) printTextOutput() {
	if m.Config.jsonSchemaValidator != nil {
		fmt.Println(m.Output)
		return
	}
	if !isOutputTTY() {
		fmt.Println()
		return
	}
	if m.Config.Raw {
		return
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(m.Config.WordWrap))
	if err != nil {
		fmt.Println(m.Output)
		return
	}
	rendered, err := renderer.Render(m.Output)
	if err != nil {
		fmt.Println(m.Output)
		return
	}
	fmt.Println(strings.TrimRightFunc(rendered, unicode.IsSpace))
}

// ensureKey resolves the API key to use, most secure source first.
func (m Mods) ensureKey(api API, defaultEnv, docsURL string) (string, error) {
	var key string
	if api.APIKeyCmd != "" {
		args, err := shellwords.Parse(api.APIKeyCmd)
		if err != nil {
			return "", modsError{err, "Failed to parse api-key-cmd"}
		}
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput() //nolint:gosec
		if err != nil {
			return "", modsError{err, "Cannot exec api-key-cmd"}
		}
		key = strings.TrimSpace(string(out))
	}
	if key == "" && api.APIKeyEnv != "" {
		key = os.Getenv(api.APIKeyEnv)
	}
	if key == "" {
		key = api.APIKey
	}
	if key == "" {
		key = os.Getenv(defaultEnv)
	}
	if key != "" {
		return key, nil
	}
	return "", modsError{
		reason: fmt.Sprintf("%[1]s required; set the environment variable %[1]s or update %[2]s in %[3]s.", m.Styles.InlineCode.Render(defaultEnv), m.Styles.InlineCode.Render("henji.yml"), m.Styles.InlineCode.Render(settingsPathHint(m.Config))),
		err:    newUserErrorf("You can grab one at %s", m.Styles.Link.Render(docsURL)),
	}
}

func settingsPathHint(cfg *Config) string {
	if cfg != nil && cfg.SettingsPath != "" {
		return cfg.SettingsPath
	}
	return defaultConfig().SettingsPath
}

func removeWhitespace(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

var tokenErrRe = regexp.MustCompile(`This model's maximum context length is (\d+) tokens. However, your messages resulted in (\d+) tokens`)

func cutPrompt(msg, prompt string) string {
	found := tokenErrRe.FindStringSubmatch(msg)
	if len(found) != 3 { //nolint:mnd
		return prompt
	}
	maxt, _ := strconv.Atoi(found[1])
	current, _ := strconv.Atoi(found[2])
	if maxt > current {
		return prompt
	}
	reduceBy := 10 + (current-maxt)*4 //nolint:mnd
	if len(prompt) > reduceBy {
		return prompt[:len(prompt)-reduceBy]
	}
	return prompt
}

func increaseIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "\t" + lines[i]
	}
	return strings.Join(lines, "\n")
}

func findModelInOtherAPIs(apis []API, excludeAPI, model string) (apiName, modelName string, found bool) {
	for _, api := range apis {
		if api.Name == excludeAPI {
			continue
		}
		for name, mod := range api.Models {
			if name == model || slices.Contains(mod.Aliases, model) {
				return api.Name, name, true
			}
		}
	}
	return "", "", false
}

func (m *Mods) resolveModel(cfg *Config) (API, Model, error) {
	for _, api := range cfg.APIs {
		if api.Name != cfg.API && cfg.API != "" {
			continue
		}
		for name, mod := range api.Models {
			if name == cfg.Model || slices.Contains(mod.Aliases, cfg.Model) {
				cfg.Model = name
				break
			}
		}
		mod, ok := api.Models[cfg.Model]
		if ok {
			mod.Name = cfg.Model
			mod.API = api.Name
			return api, mod, nil
		}
		if cfg.API != "" {
			errMsg := fmt.Sprintf("Available models are: %s", strings.Join(slices.Collect(maps.Keys(api.Models)), ", "))
			if otherAPI, otherModel, found := findModelInOtherAPIs(cfg.APIs, api.Name, cfg.Model); found {
				errMsg += fmt.Sprintf("\nTry: %s %s %s", m.Styles.InlineCode.Render("-a "+otherAPI), m.Styles.InlineCode.Render("-m "+otherModel), m.Styles.Comment.Render("(found in another configured API)"))
			}
			return API{}, Model{}, modsError{err: newUserErrorf("%s", errMsg), reason: fmt.Sprintf("The API endpoint %s does not contain the model %s", m.Styles.InlineCode.Render(cfg.API), m.Styles.InlineCode.Render(cfg.Model))}
		}
	}
	return API{}, Model{}, modsError{
		reason: fmt.Sprintf("Model %s is not in the settings file.", m.Styles.InlineCode.Render(cfg.Model)),
		err:    newUserErrorf("Please specify an API endpoint with %s or configure the model in %s", m.Styles.InlineCode.Render("--api"), m.Styles.InlineCode.Render(settingsPathHint(cfg))),
	}
}

type number interface{ int64 | float64 }

func ptrOrNil[T number](t T) *T {
	if t < 0 {
		return nil
	}
	return &t
}

type retryRequest struct {
	content string
	err     modsError
}

func (r retryRequest) Error() string { return r.err.Error() }
func (r retryRequest) Unwrap() error { return r.err }

func (m *Mods) retry(content string, err modsError) error {
	m.retries++
	if m.retries >= m.Config.MaxRetries {
		return err
	}
	time.Sleep(time.Millisecond * 100 * time.Duration(1<<m.retries)) //nolint:mnd
	return retryRequest{content: content, err: err}
}
