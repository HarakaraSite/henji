package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/openai/openai-go/v3"
)

// apiErrorDetails preserves Henji's API diagnostics across the SDK upgrade.
// SDK v3's Error() only reports the status; v1 also included the URL and
// provider's error JSON, used by stderr and --output json.
type apiErrorDetails struct {
	err *openai.Error
}

func (e apiErrorDetails) Error() string {
	if e.err.Request == nil || e.err.Response == nil {
		return e.err.Error()
	}
	return fmt.Sprintf("%s %q: %d %s %s", e.err.Request.Method, e.err.Request.URL,
		e.err.Response.StatusCode, http.StatusText(e.err.Response.StatusCode), e.err.RawJSON())
}

func (e apiErrorDetails) Unwrap() error { return e.err }

func (m *Mods) handleRequestError(err error, mod Model, content string) error {
	ae := &openai.Error{}
	if errors.As(err, &ae) {
		return m.handleAPIError(ae, mod, content)
	}
	return modsError{err, fmt.Sprintf(
		"There was a problem with the %s API request.",
		mod.API,
	)}
}

func (m *Mods) handleAPIError(err *openai.Error, mod Model, content string) error {
	cfg := m.Config
	details := apiErrorDetails{err: err}
	switch err.StatusCode {
	case http.StatusNotFound:
		if mod.Fallback != "" {
			m.Config.Model = mod.Fallback
			return m.retry(content, modsError{
				err:    details,
				reason: fmt.Sprintf("%s API server error.", mod.API),
			})
		}
		return modsError{err: details, reason: fmt.Sprintf(
			"Missing model '%s' for API '%s'.",
			cfg.Model,
			cfg.API,
		)}
	case http.StatusBadRequest:
		if err.Code == "context_length_exceeded" {
			pe := modsError{err: details, reason: "Maximum prompt size exceeded."}
			if cfg.NoLimit {
				return pe
			}

			return m.retry(cutPrompt(err.Message, content), pe)
		}
		// bad request (do not retry)
		return modsError{err: details, reason: fmt.Sprintf("%s API request error.", mod.API)}
	case http.StatusUnauthorized:
		// invalid auth or key (do not retry)
		return modsError{err: details, reason: fmt.Sprintf("Invalid %s API key.", mod.API)}
	case http.StatusTooManyRequests:
		// rate limiting or engine overload (wait and retry)
		return m.retry(content, modsError{
			err: details, reason: fmt.Sprintf("You’ve hit your %s API rate limit.", mod.API),
		})
	case http.StatusInternalServerError:
		if mod.API == "openai" {
			return m.retry(content, modsError{err: details, reason: "OpenAI API server error."})
		}
		return modsError{err: details, reason: fmt.Sprintf(
			"Error loading model '%s' for API '%s'.",
			mod.Name,
			mod.API,
		)}
	default:
		return m.retry(content, modsError{err: details, reason: "Unknown API error."})
	}
}
