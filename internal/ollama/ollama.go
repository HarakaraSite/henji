// Package ollama implements [stream.Stream] for Ollama.
package ollama

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
	"github.com/ollama/ollama/api"
)

var _ stream.Client = &Client{}

// Config represents the configuration for the Ollama API client.
type Config struct {
	BaseURL            string
	HTTPClient         *http.Client
	EmptyMessagesLimit uint
}

// DefaultConfig returns the default configuration for the Ollama API client.
func DefaultConfig() Config {
	return Config{
		BaseURL:    "http://localhost:11434/",
		HTTPClient: &http.Client{},
	}
}

// Client ollama client.
type Client struct {
	*api.Client
}

// New creates a new [Client] with the given [Config].
func New(config Config) (*Client, error) {
	u, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	client := api.NewClient(u, config.HTTPClient)
	return &Client{
		Client: client,
	}, nil
}

// Request implements stream.Client.
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	b := true
	s := &Stream{
		toolCall: request.ToolCaller,
		messages: request.Messages,
	}
	body := api.ChatRequest{
		Model:    request.Model,
		Messages: fromProtoMessages(request.Messages),
		Stream:   &b,
		Tools:    fromMCPTools(request.Tools),
		Options:  map[string]any{},
	}

	if len(request.Stop) > 0 {
		body.Options["stop"] = request.Stop[0]
	}
	if request.MaxTokens != nil {
		body.Options["num_ctx"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		body.Options["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		body.Options["top_p"] = *request.TopP
	}
	s.request = body
	s.factory = func() {
		if s.cancelFn != nil {
			s.cancelFn()
		}
		s.done = false
		s.err = nil
		ch := make(chan api.ChatResponse)
		errCh := make(chan error, 1)
		s.respCh = ch
		s.errCh = errCh
		ctx2, cancel := context.WithCancel(ctx)
		s.cancelFn = cancel
		go func() {
			defer close(ch)
			fn := func(resp api.ChatResponse) error {
				ch <- resp
				return nil
			}
			if err := c.Chat(ctx2, &s.request, fn); err != nil {
				errCh <- err
			}
		}()
	}
	s.factory()
	return s
}

// Stream ollama stream.
type Stream struct {
	request   api.ChatRequest
	err       error
	done      bool
	finalized bool
	factory   func()
	cancelFn  context.CancelFunc
	respCh    chan api.ChatResponse
	errCh     chan error
	message   api.Message
	toolCall  func(name string, data []byte) (string, error)
	messages  []proto.Message
}

// CallTools implements stream.Stream.
func (s *Stream) CallTools() []proto.ToolCallStatus {
	statuses := make([]proto.ToolCallStatus, 0, len(s.message.ToolCalls))
	for _, call := range s.message.ToolCalls {
		msg, status := stream.CallTool(
			strconv.Itoa(call.Function.Index),
			call.Function.Name,
			[]byte(call.Function.Arguments.String()),
			s.toolCall,
		)
		s.request.Messages = append(s.request.Messages, fromProtoMessage(msg))
		s.messages = append(s.messages, msg)
		statuses = append(statuses, status)
	}
	return statuses
}

// Close implements stream.Stream.
func (s *Stream) Close() error {
	s.cancelFn()
	return nil
}

// Current implements stream.Stream.
//
// Blocks until the next chunk arrives or the stream ends. Safe to call from a
// goroutine (tea.Cmd) — does not block the Bubble Tea Update loop.
func (s *Stream) Current() (proto.Chunk, error) {
	resp, ok := <-s.respCh
	if !ok {
		// goroutine exited; drain error if any (happens-before guaranteed by channel close)
		select {
		case s.err = <-s.errCh:
		default:
		}
		s.done = true
		return proto.Chunk{}, stream.ErrNoContent
	}
	chunk := proto.Chunk{
		Content: resp.Message.Content,
	}
	if resp.Message.Role != "" {
		s.message.Role = resp.Message.Role
	}
	s.message.Content += resp.Message.Content
	s.message.ToolCalls = append(s.message.ToolCalls, resp.Message.ToolCalls...)
	if resp.Done {
		s.done = true
	}
	return chunk, nil
}

// Err implements stream.Stream.
func (s *Stream) Err() error { return s.err }

// Messages implements stream.Stream.
func (s *Stream) Messages() []proto.Message { return s.messages }

// Next implements stream.Stream.
func (s *Stream) Next() bool {
	if s.done {
		if s.err != nil {
			return false
		}
		if !s.finalized {
			s.messages = append(s.messages, toProtoMessage(s.message))
			s.request.Messages = append(s.request.Messages, s.message)
			s.finalized = true
			return false
		}
		s.done = false
		s.finalized = false
		s.message = api.Message{}
		s.factory()
	}
	return true
}
