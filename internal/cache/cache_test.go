package cache

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestFailedWritePreservesExistingBody(t *testing.T) {
	c, err := New[string](t.TempDir(), ConversationCache)
	require.NoError(t, err)
	require.NoError(t, c.Write("saved", func(w io.Writer) error {
		_, err := io.WriteString(w, "original body")
		return err
	}))
	writeErr := errors.New("encoding failed")
	err = c.Write("saved", func(w io.Writer) error {
		_, err := io.WriteString(w, "partial replacement")
		require.NoError(t, err)
		return writeErr
	})
	require.ErrorIs(t, err, writeErr)
	body, err := os.ReadFile(filepath.Join(c.dir(), "saved.gob"))
	require.NoError(t, err)
	require.Equal(t, "original body", string(body))
	entries, err := os.ReadDir(c.dir())
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestFailedCommitRetainsBackupWhenRestoreFails(t *testing.T) {
	c, err := New[string](t.TempDir(), ConversationCache)
	require.NoError(t, err)
	target := filepath.Join(c.dir(), "saved.gob")
	require.NoError(t, os.WriteFile(target, []byte("original body"), 0o600))
	commitErr := errors.New("database update failed")
	err = c.WriteWithCommit("saved", func(w io.Writer) error {
		_, err := io.WriteString(w, "replacement")
		return err
	}, func() error {
		// Inject an OS failure restoring the original file after DB failure.
		require.NoError(t, os.Remove(target))
		require.NoError(t, os.Mkdir(target, 0o700))
		return commitErr
	})
	require.ErrorIs(t, err, commitErr)
	entries, readErr := os.ReadDir(c.dir())
	require.NoError(t, readErr)
	var backup string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".henji-cache-") {
			backup = filepath.Join(c.dir(), entry.Name())
		}
	}
	require.NotEmpty(t, backup)
	require.Contains(t, err.Error(), backup)
	body, readErr := os.ReadFile(backup)
	require.NoError(t, readErr)
	require.Equal(t, "original body", string(body))
}

func TestCache(t *testing.T) {
	t.Run("read non-existent", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		err = cache.Read("super-fake", &[]proto.Message{})
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("write", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		messages := []proto.Message{
			{
				Role:    proto.RoleUser,
				Content: "first 4 natural numbers",
			},
			{
				Role:    proto.RoleAssistant,
				Content: "1, 2, 3, 4",
			},
		}
		require.NoError(t, cache.Write("fake", &messages))

		result := []proto.Message{}
		require.NoError(t, cache.Read("fake", &result))

		require.ElementsMatch(t, messages, result)
	})

	t.Run("write omits attachment content", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		messages := []proto.Message{{
			Role:    proto.RoleUser,
			Content: "describe this\n\nsecret-text\n\nstdin",
			Parts: []proto.ContentPart{
				{Type: proto.ContentPartText, Text: "describe this"},
				{Type: proto.ContentPartText, Text: "secret-text", OmitFromCache: true},
				{Type: proto.ContentPartImage, Image: &proto.Image{MediaType: "image/png", Data: []byte("secret-image")}},
				{Type: proto.ContentPartText, Text: "stdin"},
			},
		}}
		require.NoError(t, cache.Write("image", &messages))

		var result []proto.Message
		require.NoError(t, cache.Read("image", &result))
		require.Len(t, result, 1)
		require.Equal(t, "describe this\n\nstdin", result[0].Content)
		require.Len(t, result[0].Parts, 2)
		require.True(t, result[0].Parts[0].TextOmitted)
		require.True(t, result[0].Parts[1].ImageOmitted)
		require.Nil(t, result[0].Parts[1].Image)
		require.NotContains(t, proto.Conversation(result).String(), "secret-text")
		require.NotContains(t, proto.Conversation(result).String(), "secret-image")
		encoded, err := os.ReadFile(filepath.Join(cache.cache.dir(), "image.gob"))
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret-text")
		require.NotContains(t, string(encoded), "secret-image")
	})

	t.Run("delete", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		cache.Write("fake", &[]proto.Message{})
		require.NoError(t, cache.Delete("fake"))
		require.ErrorIs(t, cache.Read("fake", nil), os.ErrNotExist)
	})

	t.Run("delete missing", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		require.NoError(t, cache.Delete("fake"))
	})

	t.Run("invalid id", func(t *testing.T) {
		t.Run("write", func(t *testing.T) {
			cache, err := NewConversations(t.TempDir())
			require.NoError(t, err)
			require.ErrorIs(t, cache.Write("", nil), errInvalidID)
		})
		t.Run("delete", func(t *testing.T) {
			cache, err := NewConversations(t.TempDir())
			require.NoError(t, err)
			require.ErrorIs(t, cache.Delete(""), errInvalidID)
		})
		t.Run("read", func(t *testing.T) {
			cache, err := NewConversations(t.TempDir())
			require.NoError(t, err)
			require.ErrorIs(t, cache.Read("", nil), errInvalidID)
		})
	})
}

func TestExpiringCache(t *testing.T) {
	t.Run("write and read", func(t *testing.T) {
		cache, err := NewExpiring[string](t.TempDir())
		require.NoError(t, err)

		// Write a value with expiry
		data := "test data"
		expiresAt := time.Now().Add(time.Hour).Unix()
		err = cache.Write("test", expiresAt, func(w io.Writer) error {
			_, err := w.Write([]byte(data))
			return err
		})
		require.NoError(t, err)

		// Read it back
		var result string
		err = cache.Read("test", func(r io.Reader) error {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			result = string(b)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, data, result)
	})

	t.Run("expired token", func(t *testing.T) {
		cache, err := NewExpiring[string](t.TempDir())
		require.NoError(t, err)

		// Write a value that's already expired
		data := "test data"
		expiresAt := time.Now().Add(-time.Hour).Unix() // expired 1 hour ago
		err = cache.Write("test", expiresAt, func(w io.Writer) error {
			_, err := w.Write([]byte(data))
			return err
		})
		require.NoError(t, err)

		// Try to read it
		err = cache.Read("test", func(r io.Reader) error {
			return nil
		})
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("overwrite token", func(t *testing.T) {
		cache, err := NewExpiring[string](t.TempDir())
		require.NoError(t, err)

		// Write initial value
		data1 := "test data 1"
		expiresAt1 := time.Now().Add(time.Hour).Unix()
		err = cache.Write("test", expiresAt1, func(w io.Writer) error {
			_, err := w.Write([]byte(data1))
			return err
		})
		require.NoError(t, err)

		// Write new value
		data2 := "test data 2"
		expiresAt2 := time.Now().Add(2 * time.Hour).Unix()
		err = cache.Write("test", expiresAt2, func(w io.Writer) error {
			_, err := w.Write([]byte(data2))
			return err
		})
		require.NoError(t, err)

		// Read it back - should get the new value
		var result string
		err = cache.Read("test", func(r io.Reader) error {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			result = string(b)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, data2, result)
	})
}
