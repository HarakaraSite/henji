package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	const content = "just text"
	t.Run("normal msg", func(t *testing.T) {
		msg, err := loadMsg(content)
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "foo.txt")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		msg, err := loadMsg("file://" + path)
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	t.Run("http url", func(t *testing.T) {
		withTestTransport(t, content)

		msg, err := loadMsg("http://example.test/message")
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	t.Run("https url", func(t *testing.T) {
		withTestTransport(t, content)

		msg, err := loadMsg("https://example.test/message")
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func withTestTransport(t *testing.T, content string) {
	t.Helper()

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(content)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
}
