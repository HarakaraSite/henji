package openai

import (
	"net/http"
	"strconv"
	"time"

	"github.com/openai/openai-go/v3/option"
)

// preserveLegacyRetryAfter keeps SDK v1's retry delay policy: honor the first
// parseable hint only when it is nonnegative and less than one minute.
// SDK v3 otherwise waits up to two minutes, or stops retrying above that limit.
// Removing both hints lets the SDK use its ordinary exponential backoff.
func preserveLegacyRetryAfter(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	resp, err := next(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if !legacyRetryAfterAccepted(resp.Header) {
		resp.Header.Del("Retry-After-Ms")
		resp.Header.Del("Retry-After")
	}
	return resp, nil
}

func legacyRetryAfterAccepted(header http.Header) bool {
	for _, hint := range []struct {
		name string
		unit time.Duration
	}{
		{"Retry-After-Ms", time.Millisecond},
		{"Retry-After", time.Second},
	} {
		value := header.Get(hint.name)
		if value == "" {
			continue
		}
		var delay time.Duration
		if number, err := strconv.ParseFloat(value, 64); err == nil {
			delay = time.Duration(number * float64(hint.unit))
		} else if hint.name == "Retry-After" {
			date, err := time.Parse(time.RFC1123, value)
			if err != nil {
				continue
			}
			delay = time.Until(date)
		} else {
			continue
		}
		return delay >= 0 && delay < time.Minute
	}
	return false
}
