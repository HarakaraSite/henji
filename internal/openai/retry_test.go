package openai

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLegacyRetryAfterHeaders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		headers  map[string]string
		accepted bool
	}{
		{"zero", map[string]string{"Retry-After": "0"}, true},
		{"fractional seconds", map[string]string{"Retry-After": "0.25"}, true},
		{"below minute", map[string]string{"Retry-After": "59.999"}, true},
		{"minute", map[string]string{"Retry-After": "60"}, false},
		{"milliseconds below minute", map[string]string{"Retry-After-Ms": "59999"}, true},
		{"milliseconds at minute", map[string]string{"Retry-After-Ms": "60000"}, false},
		{"milliseconds take precedence", map[string]string{"Retry-After-Ms": "10", "Retry-After": "120"}, true},
		{"long milliseconds override short seconds", map[string]string{"Retry-After-Ms": "60000", "Retry-After": "0.25"}, false},
		{"negative milliseconds override short seconds", map[string]string{"Retry-After-Ms": "-1", "Retry-After": "0.25"}, false},
		{"invalid milliseconds fall through", map[string]string{"Retry-After-Ms": "invalid", "Retry-After": "0.25"}, true},
		{"short HTTP date", map[string]string{"Retry-After": time.Now().Add(30 * time.Second).UTC().Format(time.RFC1123)}, true},
		{"long HTTP date", map[string]string{"Retry-After": time.Now().Add(2 * time.Minute).UTC().Format(time.RFC1123)}, false},
		{"past HTTP date", map[string]string{"Retry-After": time.Now().Add(-time.Minute).UTC().Format(time.RFC1123)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := make(http.Header)
			for name, value := range tc.headers {
				header.Set(name, value)
			}
			header.Set("Content-Type", "application/json")
			resp, err := preserveLegacyRetryAfter(nil, func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: header}, nil
			})
			require.NoError(t, err)
			for name, value := range tc.headers {
				if tc.accepted {
					require.Equal(t, value, resp.Header.Get(name))
				} else {
					require.Empty(t, resp.Header.Get(name))
				}
			}
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		})
	}
}
