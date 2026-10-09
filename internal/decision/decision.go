// Package decision sends native Decisions API requests to OpenAI or OpenRouter.
package decision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
)

const (
	ProtocolOpenAI     = "openai"
	ProtocolOpenRouter = "openrouter"

	openAIDefaultBaseURL     = "https://api.openai.com/v1"
	openRouterDefaultBaseURL = "https://openrouter.ai/api/alpha"
)

// Config contains the provider and transport settings for a Decisions API call.
type Config struct {
	Protocol   string
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	MaxRetries int
}

// Request is one decision request. Questions stays in the provider's native JSON
// format so fields unknown to this package pass through unchanged.
type Request struct {
	Model     string
	Text      string
	Questions json.RawMessage
	Image     *proto.Image
}

// Execute sends one native Decisions API request and returns its complete JSON
// response. MaxRetries counts attempts after the initial request.
func Execute(ctx context.Context, cfg Config, req Request) (json.RawMessage, error) {
	if ctx == nil {
		return nil, errors.New("decision: context is nil")
	}
	if cfg.Protocol != ProtocolOpenAI && cfg.Protocol != ProtocolOpenRouter {
		return nil, fmt.Errorf("decision: unsupported protocol %q", cfg.Protocol)
	}
	if cfg.MaxRetries < 0 {
		return nil, errors.New("decision: MaxRetries must be zero or greater")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("decision: API key is empty")
	}
	if err := ValidateQuestions(cfg.Protocol, req.Questions); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL(cfg.Protocol)
	}
	if err := validateBaseURL(baseURL); err != nil {
		return nil, fmt.Errorf("decision: invalid base URL: %w", err)
	}

	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var response json.RawMessage
		var err error
		if cfg.Protocol == ProtocolOpenAI {
			response, err = executeOpenAI(ctx, cfg, baseURL, req)
		} else {
			response, err = executeOpenRouter(ctx, cfg, baseURL, req)
		}
		if err == nil {
			return response, nil
		}
		if attempt >= cfg.MaxRetries || !retryable(err) {
			return nil, err
		}
		if err := waitBeforeRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}
}

// ValidateQuestions checks the JSON envelope needed to send native questions.
// Provider-specific question semantics remain the provider's responsibility.
func ValidateQuestions(protocol string, questions json.RawMessage) error {
	if protocol != ProtocolOpenAI && protocol != ProtocolOpenRouter {
		return fmt.Errorf("decision: unsupported protocol %q", protocol)
	}
	if !utf8.Valid(questions) {
		return errors.New("decision: questions must be valid UTF-8 JSON")
	}
	if !json.Valid(questions) {
		return errors.New("decision: questions must contain one valid JSON value")
	}

	switch protocol {
	case ProtocolOpenAI:
		var list []json.RawMessage
		if err := json.Unmarshal(questions, &list); err != nil || list == nil {
			return errors.New("decision: OpenAI questions must be a non-empty array")
		}
		if len(list) == 0 {
			return errors.New("decision: OpenAI questions must be a non-empty array")
		}
		for i, question := range list {
			if !isJSONObject(question) {
				return fmt.Errorf("decision: OpenAI question %d must be an object", i+1)
			}
		}
	case ProtocolOpenRouter:
		var named map[string]json.RawMessage
		if err := json.Unmarshal(questions, &named); err != nil || named == nil {
			return errors.New("decision: OpenRouter questions must be a non-empty object")
		}
		if len(named) == 0 {
			return errors.New("decision: OpenRouter questions must be a non-empty object")
		}
		for name, question := range named {
			if !isJSONObject(question) {
				return fmt.Errorf("decision: OpenRouter question %q must be an object", name)
			}
		}
	}
	return nil
}

func isJSONObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return false
	}
	return object != nil
}

func executeOpenAI(ctx context.Context, cfg Config, baseURL string, req Request) (json.RawMessage, error) {
	capture := &responseCapture{}
	options := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(0),
		option.WithMiddleware(captureResponse(capture)),
	}
	options = append(options, optionalHTTPClient(cfg.HTTPClient)...)
	client := openai.NewDecisionService(options...)

	input := openai.DecisionNewParamsInputUnion{}
	if req.Image == nil {
		input.OfString = param.NewOpt(req.Text)
	} else {
		parts := make([]openai.DecisionInputPartUnionParam, 0, 2)
		if req.Text != "" {
			parts = append(parts, openai.DecisionInputPartParamOfInputText(req.Text))
		}
		imageURL := dataURL(req.Image.MediaType, req.Image.Data)
		parts = append(parts, openai.DecisionInputPartParamOfInputImage(imageURL))
		input.OfDecisionInputMessageArray = []openai.DecisionInputMessageParam{{
			Content: openai.DecisionInputMessageContentUnionParam{OfParts: parts},
		}}
	}

	params := openai.DecisionNewParams{
		Model: req.Model,
		Input: input,
	}
	decision, err := client.New(ctx, params, option.WithJSONSet("questions", json.RawMessage(req.Questions)))
	if err != nil {
		if capture.status >= 400 {
			return nil, responseError(ProtocolOpenAI, cfg.APIKey, capture.status, capture.body)
		}
		if capture.status != 0 {
			return nil, fmt.Errorf("OpenAI Decisions API response was invalid (HTTP %d): %s: %s",
				capture.status, redact(err.Error(), cfg.APIKey), redact(string(capture.body), cfg.APIKey))
		}
		return nil, &requestError{
			message: "OpenAI Decisions API request failed: " + redact(err.Error(), cfg.APIKey),
			cause:   err,
		}
	}
	if decision == nil {
		return nil, errors.New("OpenAI Decisions API returned an empty response")
	}
	// The SDK decodes one JSON value without checking for trailing data. Validate
	// the entire HTTP body before accepting RawJSON, including its UTF-8 bytes.
	if len(bytes.TrimSpace(capture.body)) == 0 || !utf8.Valid(capture.body) || !json.Valid(capture.body) {
		return nil, fmt.Errorf("OpenAI Decisions API returned an empty or invalid JSON response (HTTP %d): %s",
			capture.status, redact(string(capture.body), cfg.APIKey))
	}
	raw := []byte(decision.RawJSON())
	if len(bytes.TrimSpace(raw)) == 0 || !utf8.Valid(raw) || !json.Valid(raw) {
		return nil, fmt.Errorf("OpenAI Decisions API returned an empty or invalid JSON response (HTTP %d): %s",
			capture.status, redact(string(capture.body), cfg.APIKey))
	}
	return json.RawMessage(raw), nil
}

func executeOpenRouter(ctx context.Context, cfg Config, baseURL string, req Request) (json.RawMessage, error) {
	state, err := openRouterState(req)
	if err != nil {
		return nil, fmt.Errorf("decision: encode OpenRouter state: %w", err)
	}
	body, err := json.Marshal(struct {
		Model     string          `json:"model"`
		State     json.RawMessage `json:"state"`
		Questions json.RawMessage `json:"questions"`
	}{Model: req.Model, State: state, Questions: req.Questions})
	if err != nil {
		return nil, fmt.Errorf("decision: encode OpenRouter request: %w", err)
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/decisions"
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	forAttempt := func() (json.RawMessage, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("decision: create OpenRouter request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")
		response, err := client.Do(httpReq)
		if err != nil {
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			return nil, &requestError{
				message: "OpenRouter Decisions API request failed: " + redact(err.Error(), cfg.APIKey),
				cause:   err,
			}
		}
		defer response.Body.Close()
		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, &requestError{
				status: response.StatusCode,
				message: fmt.Sprintf("OpenRouter Decisions API response read failed (HTTP %d): %s",
					response.StatusCode, redact(err.Error(), cfg.APIKey)),
				cause: err,
			}
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, responseError(ProtocolOpenRouter, cfg.APIKey, response.StatusCode, responseBody)
		}
		if len(bytes.TrimSpace(responseBody)) == 0 || !utf8.Valid(responseBody) || !json.Valid(responseBody) {
			return nil, fmt.Errorf("OpenRouter Decisions API returned an empty or invalid JSON response (HTTP %d): %s",
				response.StatusCode, redact(string(responseBody), cfg.APIKey))
		}
		return json.RawMessage(responseBody), nil
	}

	return forAttempt()
}

func openRouterState(req Request) (json.RawMessage, error) {
	if req.Image == nil {
		return json.Marshal(req.Text)
	}
	state := make([]any, 0, 2)
	if req.Text != "" {
		state = append(state, req.Text)
	}
	state = append(state, map[string]any{
		"type": "image_url",
		"image_url": map[string]string{
			"url": dataURL(req.Image.MediaType, req.Image.Data),
		},
	})
	return json.Marshal(state)
}

func dataURL(mediaType string, data []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func captureResponse(capture *responseCapture) option.Middleware {
	return func(request *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		response, err := next(request)
		if response == nil || response.Body == nil {
			return response, err
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		capture.status = response.StatusCode
		capture.body = body
		return response, err
	}
}

type responseCapture struct {
	status int
	body   []byte
}

func optionalHTTPClient(client *http.Client) []option.RequestOption {
	if client == nil {
		return nil
	}
	return []option.RequestOption{option.WithHTTPClient(client)}
}

func responseError(protocol, apiKey string, status int, body []byte) error {
	return &requestError{
		status: status,
		message: fmt.Sprintf("%s Decisions API returned HTTP %d %s: %s", protocolName(protocol), status,
			http.StatusText(status), redact(string(body), apiKey)),
	}
}

func protocolName(protocol string) string {
	if protocol == ProtocolOpenAI {
		return "OpenAI"
	}
	return "OpenRouter"
}

func redact(value, secret string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}

func defaultBaseURL(protocol string) string {
	if protocol == ProtocolOpenAI {
		return openAIDefaultBaseURL
	}
	return openRouterDefaultBaseURL
}

func validateBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("must include a host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("must not include a query or fragment")
	}
	return nil
}

func retryable(err error) bool {
	var requestErr *requestError
	if errors.As(err, &requestErr) {
		if requestErr.status != 0 {
			if retryableStatus(requestErr.status) {
				return true
			}
			if requestErr.status >= http.StatusBadRequest {
				return false
			}
		}
		if requestErr.cause != nil {
			return retryableNetworkError(requestErr.cause)
		}
		return false
	}
	return retryableNetworkError(err)
}

func retryableNetworkError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return (errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusConflict ||
		status == http.StatusTooManyRequests ||
		(status >= http.StatusInternalServerError && status < 600)
}

type requestError struct {
	status  int
	message string
	cause   error
}

func (e *requestError) Error() string { return e.message }
func (e *requestError) Unwrap() error { return e.cause }

func waitBeforeRetry(ctx context.Context, attempt int) error {
	delay := 100 * time.Millisecond
	for i := 0; i < attempt && delay < time.Second; i++ {
		delay *= 2
	}
	if delay > time.Second {
		delay = time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
