package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"forge.harakara.site/littleisland/henji/internal/anthropic"
	"forge.harakara.site/littleisland/henji/internal/cache"
	"forge.harakara.site/littleisland/henji/internal/google"
	"forge.harakara.site/littleisland/henji/internal/openai"
	"forge.harakara.site/littleisland/henji/internal/proto"
	"forge.harakara.site/littleisland/henji/internal/stream"
	"github.com/caarlos0/go-shellwords"
	"github.com/charmbracelet/x/exp/ordered"
)

type state int

const (
	startState state = iota
	configLoadedState
	requestState
	responseState
	doneState
	errorState
)

// Mods is the Bubble Tea model that manages reading stdin and querying the
// OpenAI API.
type Mods struct {
	Output        string
	Input         string
	Styles        styles
	Error         *modsError
	state         state
	retries       int
	schemaRetries int
	glam          *glamour.TermRenderer
	glamViewport  viewport.Model
	glamOutput    string
	glamHeight    int
	messages      []proto.Message
	mcpPool       *mcpClientPool
	anim          tea.Model
	width         int
	height        int

	db     *convoDB
	cache  *cache.Conversations
	Config *Config

	content      []string
	contentMutex *sync.Mutex

	ctx context.Context
}

func newMods(
	ctx context.Context,
	cfg *Config,
	db *convoDB,
	cache *cache.Conversations,
) *Mods {
	gr, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(cfg.WordWrap),
	)
	vp := viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))
	vp.GotoBottom()
	return &Mods{
		Styles:       stderrStyles(),
		glam:         gr,
		state:        startState,
		glamViewport: vp,
		contentMutex: &sync.Mutex{},
		db:           db,
		cache:        cache,
		Config:       cfg,
		ctx:          ctx,
		mcpPool:      newMCPClientPool(),
	}
}

// completionInput is a tea.Msg that wraps the content read from stdin.
type completionInput struct {
	content string
}

// completionOutput a tea.Msg that wraps the content returned from openai.
type completionOutput struct {
	content       string
	stream        stream.Stream
	errh          func(error) tea.Msg
	toolCallRound int // counts completed tool call rounds for MaxToolCalls enforcement
	// retryFn re-issues the request with a correction message appended,
	// used when the response fails --json-schema validation.
	retryFn func(correction string) tea.Msg
}

// Init implements tea.Model.
func (m *Mods) Init() tea.Cmd {
	if !shouldRequestBackgroundColor(isOutputTTY(), m.Config) {
		return m.findCacheOpsDetails()
	}
	return tea.Batch(m.findCacheOpsDetails(), tea.RequestBackgroundColor)
}

func shouldRequestBackgroundColor(outputTTY bool, cfg *Config) bool {
	return outputTTY && !cfg.Raw && cfg.Output != "json" && cfg.jsonSchemaValidator == nil
}

// Update implements tea.Model.
func (m *Mods) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case cacheDetailsMsg:
		m.Config.cacheWriteToID = msg.WriteID
		m.Config.cacheWriteToTitle = msg.Title
		m.Config.cacheReadFromID = msg.ReadID
		m.Config.API = msg.API
		m.Config.Model = msg.Model

		if !m.Config.Quiet {
			m.anim = newAnim(defaultFanciness, defaultStatusText, m.Styles)
			cmds = append(cmds, m.anim.Init())
		}
		m.state = configLoadedState
		cmds = append(cmds, m.readStdinCmd)

	case completionInput:
		if msg.content != "" {
			m.Input = removeWhitespace(msg.content)
		}
		if m.Input == "" && m.Config.Prefix == "" && m.Config.Show == "" {
			return m, m.quit
		}
		if len(m.Config.Delete) > 0 ||
			m.Config.ShowHelp ||
			m.Config.List ||
			m.Config.ListRoles ||
			m.Config.Settings {
			return m, m.quit
		}

		m.state = requestState
		cmds = append(cmds, m.startCompletionCmd(msg.content))
	case completionOutput:
		if msg.stream == nil {
			m.state = doneState
			return m, m.quit
		}
		if msg.content != "" {
			m.appendToOutput(msg.content)
			m.state = responseState
		}
		cmds = append(cmds, m.receiveCompletionStreamCmd(completionOutput{
			stream:        msg.stream,
			errh:          msg.errh,
			toolCallRound: msg.toolCallRound,
			retryFn:       msg.retryFn,
		}))
	case modsError:
		m.Error = &msg
		m.state = errorState
		return m, m.quit
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.glamViewport.SetWidth(m.width)
		m.glamViewport.SetHeight(m.height)
		return m, nil
	case tea.BackgroundColorMsg:
		m.Styles = makeStyles(m.Styles.profile, msg.IsDark())
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.state = doneState
			return m, m.quit
		}
	}
	if !m.Config.Quiet && (m.state == configLoadedState || m.state == requestState) {
		var cmd tea.Cmd
		m.anim, cmd = m.anim.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.viewportNeeded() {
		// Only respond to keypresses when the viewport (i.e. the content) is
		// taller than the window.
		var cmd tea.Cmd
		m.glamViewport, cmd = m.glamViewport.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m Mods) viewportNeeded() bool {
	return m.glamHeight > m.height
}

// View implements tea.Model.
func (m *Mods) View() tea.View {
	//nolint:exhaustive
	switch m.state {
	case errorState:
		return tea.NewView("")
	case requestState:
		if !m.Config.Quiet {
			return m.anim.View()
		}
	case responseState:
		if !m.Config.Raw && isOutputTTY() {
			if m.viewportNeeded() {
				return tea.NewView(m.glamViewport.View())
			}
			// We don't need the viewport yet.
			return tea.NewView(m.glamOutput)
		}

		if isOutputTTY() && !m.Config.Raw {
			return tea.NewView(m.Output)
		}

		m.contentMutex.Lock()
		if m.Config.Output != "json" && m.Config.jsonSchemaValidator == nil {
			for _, c := range m.content {
				fmt.Print(c)
			}
		}
		m.content = []string{}
		m.contentMutex.Unlock()
	case doneState:
		if !isOutputTTY() && m.Config.Output != "json" && m.Config.jsonSchemaValidator == nil {
			fmt.Printf("\n")
		}
		return tea.NewView("")
	}
	return tea.NewView("")
}

func (m *Mods) quit() tea.Msg {
	m.mcpPool.closeAll()
	return tea.Quit()
}

func (m *Mods) retry(content string, err modsError) tea.Msg {
	m.retries++
	if m.retries >= m.Config.MaxRetries {
		return err
	}
	wait := time.Millisecond * 100 * time.Duration(math.Pow(2, float64(m.retries))) //nolint:mnd
	time.Sleep(wait)
	return completionInput{content}
}

func (m *Mods) startCompletionCmd(content string) tea.Cmd {
	if m.Config.Show != "" {
		return m.readFromCache()
	}

	return func() tea.Msg {
		var mod Model
		var api API
		var ccfg openai.Config
		var accfg anthropic.Config
		var gccfg google.Config

		cfg := m.Config
		api, mod, err := m.resolveModel(cfg)
		cfg.API = mod.API
		if err != nil {
			return err
		}
		if api.Name == "" {
			eps := make([]string, 0)
			for _, a := range cfg.APIs {
				eps = append(eps, m.Styles.InlineCode.Render(a.Name))
			}
			return modsError{
				err: newUserErrorf(
					"Your configured API endpoints are: %s",
					eps,
				),
				reason: fmt.Sprintf(
					"The API endpoint %s is not configured.",
					m.Styles.InlineCode.Render(cfg.API),
				),
			}
		}

		switch mod.API {
		case "anthropic":
			key, err := m.ensureKey(api, "ANTHROPIC_API_KEY", "https://console.anthropic.com/settings/keys")
			if err != nil {
				return modsError{err, "Anthropic authentication failed"}
			}
			accfg = anthropic.DefaultConfig(key)
			if api.BaseURL != "" {
				accfg.BaseURL = api.BaseURL
			}
		case "google":
			key, err := m.ensureKey(api, "GOOGLE_API_KEY", "https://aistudio.google.com/app/apikey")
			if err != nil {
				return modsError{err, "Google authentication failed"}
			}
			gccfg = google.DefaultConfig(mod.Name, key)
			gccfg.ThinkingBudget = mod.ThinkingBudget
		case "azure", "azure-ad": //nolint:goconst
			key, err := m.ensureKey(api, "AZURE_OPENAI_KEY", "https://aka.ms/oai/access")
			if err != nil {
				return modsError{err, "Azure authentication failed"}
			}
			ccfg = openai.Config{
				AuthToken: key,
				BaseURL:   api.BaseURL,
			}
			if mod.API == "azure-ad" {
				ccfg.APIType = "azure-ad"
			}
			if api.User != "" {
				cfg.User = api.User
			}
		default:
			key, err := m.ensureKey(api, "OPENAI_API_KEY", "https://platform.openai.com/account/api-keys")
			if err != nil {
				return modsError{err, "OpenAI authentication failed"}
			}
			ccfg = openai.Config{
				AuthToken: key,
				BaseURL:   api.BaseURL,
			}
		}

		if cfg.HTTPProxy != "" {
			proxyURL, err := url.Parse(cfg.HTTPProxy)
			if err != nil {
				return modsError{err, "There was an error parsing your proxy URL."}
			}
			httpClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
			ccfg.HTTPClient = httpClient
			accfg.HTTPClient = httpClient
			gccfg.HTTPClient = httpClient
		}

		if mod.MaxChars == 0 {
			mod.MaxChars = cfg.MaxInputChars
		}
		// Model-level max-completion-tokens overrides the global setting when set.
		if mod.MaxCompletionTokens > 0 {
			cfg.MaxCompletionTokens = mod.MaxCompletionTokens
		}

		// o1 models do not support max_tokens; use max_completion_tokens instead.
		// If the user set max-tokens but not max-completion-tokens, promote it.
		if strings.HasPrefix(mod.Name, "o1") {
			if cfg.MaxCompletionTokens == 0 && cfg.MaxTokens > 0 {
				cfg.MaxCompletionTokens = cfg.MaxTokens
			}
			cfg.MaxTokens = 0
		}

		// MCPTimeout context: only for tool listing / individual tool calls.
		// The LLM stream below uses m.ctx directly (no timeout) so long
		// responses are not cut off by the MCP deadline.
		ctx, cancel := context.WithTimeout(m.ctx, cfg.MCPTimeout)
		tools, err := m.mcpTools(ctx)
		cancel()
		if err != nil {
			return err
		}

		if err := m.setupStreamContext(content, mod); err != nil {
			return err
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
			Tools:       tools,
			ToolCaller: func(name string, data []byte) (string, error) {
				// Per-call timeout: each tool invocation is bounded independently.
				ctx, cancel := context.WithTimeout(m.ctx, cfg.MCPTimeout)
				defer cancel()
				return m.toolCall(ctx, name, data)
			},
		}
		if cfg.MaxTokens > 0 {
			request.MaxTokens = &cfg.MaxTokens
		}
		if cfg.MaxCompletionTokens > 0 {
			request.MaxCompletionTokens = &cfg.MaxCompletionTokens
		}
		request.JSONSchema = cfg.jsonSchemaDoc

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

		errh := func(err error) tea.Msg {
			return m.handleRequestError(err, mod, m.Input)
		}

		// retryFn re-issues the request with the failed assistant response
		// still in context plus a correction message, so the model sees
		// what it got wrong instead of blindly regenerating from scratch
		// (unlike m.retry, which resets the whole conversation via
		// setupStreamContext).
		var retryFn func(correction string) tea.Msg
		retryFn = func(correction string) tea.Msg {
			retryReq := request
			retryReq.Messages = append(append([]proto.Message{}, m.messages...), proto.Message{
				Role:    proto.RoleUser,
				Content: correction,
			})
			return m.receiveCompletionStreamCmd(completionOutput{
				stream:  client.Request(m.ctx, retryReq),
				errh:    errh,
				retryFn: retryFn,
			})()
		}

		stream := client.Request(m.ctx, request)
		return m.receiveCompletionStreamCmd(completionOutput{
			stream:  stream,
			errh:    errh,
			retryFn: retryFn,
		})()
	}
}

// ensureKey resolves the API key to use, most secure source first: a
// command's stdout never touches disk, a named env var is at least not
// checked into henji.yml, and a plaintext api-key is the least secure of
// the three explicit options. See README.md's "API key management" section.
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
		reason: fmt.Sprintf(
			"%[1]s required; set the environment variable %[1]s or update %[2]s through %[3]s.",
			m.Styles.InlineCode.Render(defaultEnv),
			m.Styles.InlineCode.Render("henji.yml"),
			m.Styles.InlineCode.Render("henji --settings"),
		),
		err: newUserErrorf(
			"You can grab one at %s",
			m.Styles.Link.Render(docsURL),
		),
	}
}

// checkJSONSchema validates the last assistant message against
// --json-schema. On failure it either asks the model to correct itself
// (via msg.retryFn, which keeps the failed response in context) or, once
// --json-schema-retries is exhausted, returns a terminal error.
func (m *Mods) checkJSONSchema(msg completionOutput) tea.Msg {
	var content string
	if len(m.messages) > 0 {
		content = m.messages[len(m.messages)-1].Content
	}

	err := validateAgainstSchema(m.Config.jsonSchemaValidator, content)
	if err == nil {
		return nil
	}

	m.schemaRetries++
	if m.schemaRetries > m.Config.JSONSchemaRetries {
		return modsError{
			err: err,
			reason: fmt.Sprintf(
				"Response did not match --json-schema after %d attempt(s).",
				m.schemaRetries,
			),
		}
	}

	correction := fmt.Sprintf(
		"Your previous response did not conform to the required JSON Schema: %s\n"+
			"Respond again with ONLY JSON that strictly matches the schema, and nothing else.",
		err,
	)

	// Discard the failed attempt's accumulated display/save output. m.Output
	// is appended to unconditionally in appendToOutput, so without this a
	// retry's chunks would be concatenated onto the rejected response
	// instead of replacing it (m.messages, used to build the corrected
	// request below, is unaffected and correctly keeps the failed turn).
	m.Output = ""
	m.contentMutex.Lock()
	m.content = nil
	m.contentMutex.Unlock()

	return msg.retryFn(correction)
}

func (m *Mods) receiveCompletionStreamCmd(msg completionOutput) tea.Cmd {
	return func() tea.Msg {
		if msg.stream.Next() {
			chunk, err := msg.stream.Current()
			if err != nil && !errors.Is(err, stream.ErrNoContent) {
				_ = msg.stream.Close()
				return msg.errh(err)
			}
			return completionOutput{
				content:       chunk.Content,
				stream:        msg.stream,
				errh:          msg.errh,
				toolCallRound: msg.toolCallRound,
				retryFn:       msg.retryFn,
			}
		}

		// stream is done, check for errors
		if err := msg.stream.Err(); err != nil {
			return msg.errh(err)
		}

		results := msg.stream.CallTools()
		if len(results) == 0 {
			m.messages = msg.stream.Messages()
			if m.Config.jsonSchemaValidator != nil {
				if errMsg := m.checkJSONSchema(msg); errMsg != nil {
					return errMsg
				}
			}
			return completionOutput{errh: msg.errh}
		}

		// Enforce MaxToolCalls limit (0 = unlimited)
		nextRound := msg.toolCallRound + 1
		cfg := m.Config
		if cfg.MaxToolCalls > 0 && nextRound > cfg.MaxToolCalls {
			return modsError{
				err: fmt.Errorf("tool call limit of %d reached", cfg.MaxToolCalls),
				reason: fmt.Sprintf(
					"Exceeded max-tool-calls limit of %d. Set max-tool-calls: 0 in henji.yml to disable.",
					cfg.MaxToolCalls,
				),
			}
		}

		toolMsg := completionOutput{
			stream:        msg.stream,
			errh:          msg.errh,
			toolCallRound: nextRound,
			retryFn:       msg.retryFn,
		}
		for _, call := range results {
			toolMsg.content += call.String()
		}
		return toolMsg
	}
}

type cacheDetailsMsg struct {
	WriteID, Title, ReadID, API, Model string
}

func (m *Mods) findCacheOpsDetails() tea.Cmd {
	return func() tea.Msg {
		continueLast := m.Config.ContinueLast || (m.Config.Continue != "" && m.Config.Title == "")
		readID := ordered.First(m.Config.Continue, m.Config.Show)
		writeID := ordered.First(m.Config.Title, m.Config.Continue)
		title := writeID
		model := m.Config.Model
		api := m.Config.API

		if readID != "" || continueLast {
			found, err := m.findReadID(readID)
			if err != nil {
				return modsError{
					err:    err,
					reason: "Could not find the conversation.",
				}
			}
			if found != nil {
				readID = found.ID
				if found.Model != nil && found.API != nil {
					model = *found.Model
					api = *found.API
				}
			}
		}

		// if we are continuing last, update the existing conversation
		if continueLast {
			writeID = readID
		}

		if writeID == "" {
			writeID = newConversationID()
		}

		if !sha1reg.MatchString(writeID) {
			convo, err := m.db.Find(writeID)
			if err != nil {
				// its a new conversation with a title
				writeID = newConversationID()
			} else {
				writeID = convo.ID
			}
		}

		return cacheDetailsMsg{
			WriteID: writeID,
			Title:   title,
			ReadID:  readID,
			API:     api,
			Model:   model,
		}
	}
}

func (m *Mods) findReadID(in string) (*Conversation, error) {
	convo, err := m.db.Find(in)
	if err == nil {
		return convo, nil
	}
	if errors.Is(err, errNoMatches) && m.Config.Show == "" {
		convo, err := m.db.FindHEAD()
		if err != nil {
			return nil, err
		}
		return convo, nil
	}
	return nil, err
}

func (m *Mods) readStdinCmd() tea.Msg {
	if !isInputTTY() {
		reader := bufio.NewReader(os.Stdin)
		stdinBytes, err := io.ReadAll(reader)
		if err != nil {
			return modsError{err, "Unable to read stdin."}
		}

		return completionInput{increaseIndent(string(stdinBytes))}
	}
	return completionInput{""}
}

func (m *Mods) readFromCache() tea.Cmd {
	return func() tea.Msg {
		var messages []proto.Message
		if err := m.cache.Read(m.Config.cacheReadFromID, &messages); err != nil {
			return modsError{err, "There was an error loading the conversation."}
		}

		m.appendToOutput(proto.Conversation(messages).String())
		return completionOutput{
			errh: func(err error) tea.Msg {
				return modsError{err: err}
			},
		}
	}
}

const tabWidth = 4

func (m *Mods) appendToOutput(s string) {
	m.Output += s
	if !isOutputTTY() || m.Config.Raw || m.Config.Output == "json" || m.Config.jsonSchemaValidator != nil {
		m.contentMutex.Lock()
		m.content = append(m.content, s)
		m.contentMutex.Unlock()
		return
	}

	wasAtBottom := m.glamViewport.ScrollPercent() == 1.0
	oldHeight := m.glamHeight
	m.glamOutput, _ = m.glam.Render(m.Output)
	m.glamOutput = strings.TrimRightFunc(m.glamOutput, unicode.IsSpace)
	m.glamOutput = strings.ReplaceAll(m.glamOutput, "\t", strings.Repeat(" ", tabWidth))
	m.glamHeight = lipgloss.Height(m.glamOutput)
	m.glamOutput += "\n"
	truncatedGlamOutput := lipgloss.NewStyle().
		MaxWidth(m.width).
		Render(m.glamOutput)
	m.glamViewport.SetContent(truncatedGlamOutput)
	if oldHeight < m.glamHeight && wasAtBottom {
		// If the viewport's at the bottom and we've received a new
		// line of content, follow the output by auto scrolling to
		// the bottom.
		m.glamViewport.GotoBottom()
	}
}

// if the input is whitespace only, make it empty.
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

	// 1 token =~ 4 chars
	// cut 10 extra chars 'just in case'
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
			return API{}, Model{}, modsError{
				err: newUserErrorf(
					"Available models are: %s",
					strings.Join(slices.Collect(maps.Keys(api.Models)), ", "),
				),
				reason: fmt.Sprintf(
					"The API endpoint %s does not contain the model %s",
					m.Styles.InlineCode.Render(cfg.API),
					m.Styles.InlineCode.Render(cfg.Model),
				),
			}
		}
	}

	return API{}, Model{}, modsError{
		reason: fmt.Sprintf(
			"Model %s is not in the settings file.",
			m.Styles.InlineCode.Render(cfg.Model),
		),
		err: newUserErrorf(
			"Please specify an API endpoint with %s or configure the model in the settings: %s",
			m.Styles.InlineCode.Render("--api"),
			m.Styles.InlineCode.Render("henji --settings"),
		),
	}
}

type number interface{ int64 | float64 }

func ptrOrNil[T number](t T) *T {
	if t < 0 {
		return nil
	}
	return &t
}
