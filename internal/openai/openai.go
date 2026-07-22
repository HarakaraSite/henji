// Package openai implements [stream.Stream] for OpenAI.
package openai

import (
	"context"
	"net/http"
	"strings"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"forge.harakara.site/littleisland/henji/v2/internal/stream"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/azure"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/shared"
)

var _ stream.Client = &Client{}

// Client is the openai client.
type Client struct {
	*openai.Client
}

// Config represents the configuration for the OpenAI API client.
type Config struct {
	AuthToken  string
	BaseURL    string
	HTTPClient interface {
		Do(*http.Request) (*http.Response, error)
	}
	APIType string
}

// DefaultConfig returns the default configuration for the OpenAI API client.
func DefaultConfig(authToken string) Config {
	return Config{
		AuthToken: authToken,
	}
}

// New creates a new [Client] with the given [Config].
func New(config Config) *Client {
	opts := []option.RequestOption{option.WithMiddleware(ignoreEmptySSEEvents)}

	if config.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(config.HTTPClient))
	}

	if config.APIType == "azure-ad" {
		opts = append(opts, azure.WithAPIKey(config.AuthToken))
		if config.BaseURL != "" {
			opts = append(opts, azure.WithEndpoint(config.BaseURL, "v1"))
		}
	} else {
		opts = append(opts, option.WithAPIKey(config.AuthToken))
		if config.BaseURL != "" {
			opts = append(opts, option.WithBaseURL(config.BaseURL))
		}
	}
	client := openai.NewClient(opts...)
	return &Client{
		Client: &client,
	}
}

// strictParam returns the response_format.json_schema.strict default for
// the given dialect. Real OpenAI enforces strict-mode constraints (e.g.
// additionalProperties:false, all fields required) reliably; other
// OpenAI-compatible dialects (Groq's non gpt-oss-* models, local gateways,
// Azure, ...) may reject or ignore strict mode, so leave it unset there and
// let the server fall back to its own default/best-effort behavior.
func strictParam(api string) param.Opt[bool] {
	if api == "openai" {
		return openai.Bool(true)
	}
	return param.Opt[bool]{}
}

// Request makes a new request and returns a stream.
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	body := openai.ChatCompletionNewParams{
		Model:    request.Model,
		User:     openai.String(request.User),
		Messages: fromProtoMessages(request.Messages),
	}

	if request.API != "perplexity" || !strings.Contains(request.Model, "online") {
		if request.Temperature != nil {
			body.Temperature = openai.Float(*request.Temperature)
		}
		if request.TopP != nil {
			body.TopP = openai.Float(*request.TopP)
		}
		body.Stop = openai.ChatCompletionNewParamsStopUnion{
			OfStringArray: request.Stop,
		}
		if request.MaxTokens != nil {
			body.MaxTokens = openai.Int(*request.MaxTokens)
		}
		if request.MaxCompletionTokens != nil {
			body.MaxCompletionTokens = openai.Int(*request.MaxCompletionTokens)
		}
		if request.API == "openai" && request.ResponseFormat != nil && *request.ResponseFormat == "json" {
			body.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			}
		}
	}

	// JSONSchema takes precedence over the loose ResponseFormat above and is
	// set unconditionally (outside the perplexity-online guard above): this
	// covers every OpenAI-compatible dialect, including perplexity's online
	// models. If a dialect rejects response_format outright, that surfaces
	// as a clear 400 from the API instead of silently never sending the
	// schema and leaving client-side validation to retry against a model
	// that was never actually asked to follow it.
	if request.JSONSchema != nil {
		body.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "response",
					Schema: request.JSONSchema,
					Strict: strictParam(request.API),
				},
			},
		}
	}

	s := &Stream{
		stream:   c.Chat.Completions.NewStreaming(ctx, body),
		messages: request.Messages,
	}
	return s
}

// Stream openai stream.
type Stream struct {
	done     bool
	stream   *ssestream.Stream[openai.ChatCompletionChunk]
	message  openai.ChatCompletionAccumulator
	messages []proto.Message
}

// Close implements stream.Stream.
func (s *Stream) Close() error { return s.stream.Close() } //nolint:wrapcheck

// Current implements stream.Stream.
func (s *Stream) Current() (proto.Chunk, error) {
	event := s.stream.Current()
	s.message.AddChunk(event)
	if len(event.Choices) > 0 {
		return proto.Chunk{
			Content: event.Choices[0].Delta.Content,
		}, nil
	}
	return proto.Chunk{}, stream.ErrNoContent
}

// Err implements stream.Stream.
func (s *Stream) Err() error { return s.stream.Err() } //nolint:wrapcheck

// Messages implements stream.Stream.
func (s *Stream) Messages() []proto.Message { return s.messages }

// Next implements stream.Stream. Once it returns false, it keeps returning
// false without issuing any further requests.
func (s *Stream) Next() bool {
	if s.done {
		return false
	}

	if s.stream.Next() {
		return true
	}

	s.done = true
	if len(s.message.Choices) > 0 {
		msg := s.message.Choices[0].Message.ToParam()
		s.messages = append(s.messages, toProtoMessage(msg))
	}

	return false
}
