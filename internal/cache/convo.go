package cache

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
)

// Conversations is the conversation cache.
type Conversations struct {
	cache *Cache[[]proto.Message]
}

// NewConversations creates a new conversation cache.
func NewConversations(dir string) (*Conversations, error) {
	cache, err := New[[]proto.Message](dir, ConversationCache)
	if err != nil {
		return nil, err
	}
	return &Conversations{
		cache: cache,
	}, nil
}

func (c *Conversations) Read(id string, messages *[]proto.Message) error {
	return c.cache.Read(id, func(r io.Reader) error {
		return decode(r, messages)
	})
}

func (c *Conversations) Write(id string, messages *[]proto.Message) error {
	return c.WriteWithCommit(id, messages, nil)
}

// WriteWithCommit keeps the previous body when its metadata cannot be saved.
// Concurrent callers must hold Lock from reading the history through saving it.
func (c *Conversations) WriteWithCommit(id string, messages *[]proto.Message, commitFn func() error) error {
	return c.cache.WriteWithCommit(id, func(w io.Writer) error {
		sanitized := proto.MessagesForCache(*messages)
		return encode(w, &sanitized)
	}, commitFn)
}

// Lock serializes operations on one conversation across processes. The caller
// must close the returned handle after the conversation has been saved.
func (c *Conversations) Lock(ctx context.Context, id string) (io.Closer, error) {
	return c.cache.lockWrite(ctx, id)
}

// Delete a conversation.
func (c *Conversations) Delete(id string) error {
	return c.cache.Delete(id)
}

func init() {
	gob.Register(errors.New(""))
}

func encode(w io.Writer, messages *[]proto.Message) error {
	if err := gob.NewEncoder(w).Encode(messages); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// decode decodes the given reader using gob.
// we use a teereader in case the user tries to read a message in the old
// format (from before MCP), and if so convert between types to avoid encoding
// errors.
func decode(r io.Reader, messages *[]proto.Message) error {
	var tr bytes.Buffer
	if err1 := gob.NewDecoder(io.TeeReader(r, &tr)).Decode(messages); err1 != nil {
		var noCalls []noCallMessage
		if err2 := gob.NewDecoder(&tr).Decode(&noCalls); err2 != nil {
			return fmt.Errorf("decode: %w", err1)
		}
		for _, msg := range noCalls {
			*messages = append(*messages, proto.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}
	return nil
}

// noCallMessage compatibility with messages with no tool calls.
type noCallMessage struct {
	Content string
	Role    string
}
