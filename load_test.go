package main

import (
	"fmt"
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
		withTestHTTPClient(t, http.StatusOK, content)

		msg, err := loadMsg("http://example.test/message")
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	t.Run("https url", func(t *testing.T) {
		withTestHTTPClient(t, http.StatusOK, content)

		msg, err := loadMsg("https://example.test/message")
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	t.Run("http status error", func(t *testing.T) {
		withTestHTTPClient(t, http.StatusNotFound, "not found")

		msg, err := loadMsg("https://example.test/missing")
		require.Empty(t, msg)
		require.EqualError(t, err, "load https://example.test/missing: 404 Not Found")
	})

	t.Run("http request has timeout", func(t *testing.T) {
		var hasDeadline bool
		withTestRoundTripper(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_, hasDeadline = req.Context().Deadline()
			return testHTTPResponse(req, http.StatusOK, content), nil
		}))

		msg, err := loadMsg("https://example.test/message")
		require.NoError(t, err)
		require.Equal(t, content, msg)
		require.True(t, hasDeadline)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func withTestHTTPClient(t *testing.T, status int, content string) {
	t.Helper()

	withTestRoundTripper(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testHTTPResponse(req, status, content), nil
	}))
}

func withTestRoundTripper(t *testing.T, transport http.RoundTripper) {
	t.Helper()

	originalClient := loadHTTPClient
	loadHTTPClient = &http.Client{
		Transport: transport,
		Timeout:   loadHTTPTimeout,
	}
	t.Cleanup(func() {
		loadHTTPClient = originalClient
	})
}

func testHTTPResponse(req *http.Request, status int, content string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader(content)),
		Header:     make(http.Header),
		Request:    req,
	}
}
