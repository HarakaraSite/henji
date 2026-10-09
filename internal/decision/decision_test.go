package decision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
)

const testAPIKey = "decision-test-key"

func TestExecuteNativeProtocolContracts(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		t.Run(protocol, func(t *testing.T) {
			questions := `[{"type":"predicate","name":"first","instructions":"is it?","vendor":{"flag":true}},{"type":"choice","name":"second","instructions":"Pick one.","choices":[{"value":"a","description":"Option A"},{"value":"b","description":"Option B"}],"vendor_field":[1,2]}]`
			if protocol == ProtocolOpenRouter {
				questions = `{"first":{"type":"noul","instructions":"is it?","vendor":{"flag":true}},"second":{"type":"choice","instructions":"Pick one.","criteria":{"a":"Option A","b":"Option B"},"vendor_field":[1,2]}}`
			}
			text := "  leading space\nsecond line\n"
			imageBytes := []byte{0, 1, 2, 255, '\n'}
			responseBody := `{"id":"dec-1","answers":[{"type":"refusal","name":"first","provider_extra":true}],"model":"test-model","usage":{"input_tokens":2,"output_tokens":0},"native_extra":{"kept":true}}`
			if protocol == ProtocolOpenRouter {
				responseBody = `{"id":"dec-1","answers":{"first":{"type":"refusal","reason":"provider refusal"}},"native_extra":{"kept":true}}`
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				wantPath := "/gateway/decisions"
				if protocol == ProtocolOpenAI {
					wantPath = "/gateway/v1/decisions"
				}
				if r.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
				}
				if r.Method != http.MethodPost {
					t.Errorf("method = %q, want POST", r.Method)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
					t.Errorf("Authorization = %q", got)
				}
				if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
					t.Errorf("Content-Type = %q", got)
				}
				requestBody, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(requestBody, &payload); err != nil {
					t.Errorf("request JSON: %v", err)
					return
				}
				if got := string(payload["model"]); got != `"test-model"` {
					t.Errorf("model = %s", got)
				}
				if len(payload) != 3 {
					t.Errorf("request fields = %v, want only model, input/state, questions", mapKeys(payload))
				}
				var gotQuestions any
				if err := json.Unmarshal(payload["questions"], &gotQuestions); err != nil {
					t.Errorf("questions JSON: %v", err)
				} else if strings.Contains(protocol, "openai") {
					var got []map[string]any
					if err := json.Unmarshal(payload["questions"], &got); err != nil {
						t.Errorf("OpenAI questions: %v", err)
					} else if len(got) != 2 || got[0]["vendor"].(map[string]any)["flag"] != true || got[1]["vendor_field"] == nil {
						t.Errorf("OpenAI questions lost native or unknown fields: %#v", got)
					}
				} else {
					var got map[string]map[string]any
					if err := json.Unmarshal(payload["questions"], &got); err != nil {
						t.Errorf("OpenRouter questions: %v", err)
					} else if len(got) != 2 || got["first"]["vendor"].(map[string]any)["flag"] != true || got["second"]["vendor_field"] == nil {
						t.Errorf("OpenRouter questions lost native or unknown fields: %#v", got)
					}
				}

				if protocol == ProtocolOpenAI {
					var input []struct {
						Role    string `json:"role"`
						Content []struct {
							Type     string `json:"type"`
							Text     string `json:"text"`
							ImageURL string `json:"image_url"`
						} `json:"content"`
					}
					if err := json.Unmarshal(payload["input"], &input); err != nil {
						t.Errorf("OpenAI image input: %v", err)
					} else {
						wantImage := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
						if len(input) != 1 || input[0].Role != "user" || len(input[0].Content) != 2 ||
							input[0].Content[0].Type != "input_text" || input[0].Content[0].Text != text ||
							input[0].Content[1].Type != "input_image" || input[0].Content[1].ImageURL != wantImage {
							t.Errorf("OpenAI content = %#v", input)
						}
					}
				} else {
					var state []any
					if err := json.Unmarshal(payload["state"], &state); err != nil {
						t.Errorf("OpenRouter image state: %v", err)
					} else {
						if len(state) != 2 || state[0] != text {
							t.Errorf("OpenRouter state = %#v", state)
						} else {
							image, ok := state[1].(map[string]any)
							if !ok || image["type"] != "image_url" {
								t.Errorf("OpenRouter image part = %#v", state[1])
							} else {
								imageURL := image["image_url"].(map[string]any)["url"]
								wantImage := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
								if imageURL != wantImage {
									t.Errorf("OpenRouter image URL = %v, want %q", imageURL, wantImage)
								}
							}
						}
					}
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, responseBody)
			}))
			defer server.Close()

			baseURL := strings.TrimRight(server.URL, "/") + "/gateway/"
			if protocol == ProtocolOpenAI {
				baseURL += "v1/"
			}
			got, err := Execute(context.Background(), Config{
				Protocol:   protocol,
				BaseURL:    baseURL,
				APIKey:     testAPIKey,
				HTTPClient: server.Client(),
			}, Request{
				Model:     "test-model",
				Text:      text,
				Questions: json.RawMessage(questions),
				Image:     &proto.Image{MediaType: "image/png", Data: imageBytes},
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("calls = %d, want 1", calls.Load())
			}
			if !json.Valid(got) || !bytes.Contains(got, []byte(`"native_extra":{"kept":true}`)) || !bytes.Contains(got, []byte(`"type":"refusal"`)) {
				t.Fatalf("response did not preserve raw refusal and provider fields: %s", got)
			}
			if !bytes.Equal(got, []byte(responseBody)) {
				t.Fatalf("raw response = %q, want original body %q", got, responseBody)
			}
		})
	}
}

func TestExecuteTextOnlyKeepsPlainTextInput(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		t.Run(protocol, func(t *testing.T) {
			text := "\n  exact input\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var payload map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				key := "state"
				if protocol == ProtocolOpenAI {
					key = "input"
				}
				var got string
				if err := json.Unmarshal(payload[key], &got); err != nil {
					t.Errorf("%s should be a string: %v", key, err)
				} else if got != text {
					t.Errorf("%s = %q, want %q", key, got, text)
				}
				if protocol == ProtocolOpenAI {
					_, _ = io.WriteString(w, `{"answers":[],"model":"m","usage":{}}`)
				} else {
					_, _ = io.WriteString(w, `{"answers":{},"extra":1}`)
				}
			}))
			defer server.Close()

			base := server.URL
			if protocol == ProtocolOpenAI {
				base += "/v1"
			}
			_, err := Execute(context.Background(), Config{Protocol: protocol, BaseURL: base, APIKey: testAPIKey}, Request{
				Model: "m", Text: text, Questions: questionsFor(protocol),
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	}
}

func TestValidateQuestions(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		questions json.RawMessage
		wantErr   bool
	}{
		{name: "openai native array", protocol: ProtocolOpenAI, questions: json.RawMessage(`[{"type":"future","x":1}]`)},
		{name: "openrouter named object", protocol: ProtocolOpenRouter, questions: json.RawMessage(`{"q":{"type":"future","x":1}}`)},
		{name: "openai array required", protocol: ProtocolOpenAI, questions: json.RawMessage(`{"q":{}}`), wantErr: true},
		{name: "openrouter object required", protocol: ProtocolOpenRouter, questions: json.RawMessage(`[]`), wantErr: true},
		{name: "non-empty array", protocol: ProtocolOpenAI, questions: json.RawMessage(`[]`), wantErr: true},
		{name: "non-empty object", protocol: ProtocolOpenRouter, questions: json.RawMessage(`{}`), wantErr: true},
		{name: "question object", protocol: ProtocolOpenAI, questions: json.RawMessage(`[null]`), wantErr: true},
		{name: "named question object", protocol: ProtocolOpenRouter, questions: json.RawMessage(`{"q":[]}`), wantErr: true},
		{name: "single JSON only", protocol: ProtocolOpenRouter, questions: json.RawMessage(`{"q":{}} {}`), wantErr: true},
		{name: "invalid UTF-8", protocol: ProtocolOpenRouter, questions: json.RawMessage([]byte{'{', '"', 0xff, '"', ':', '{', '}', '}'}), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuestions(tt.protocol, tt.questions)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateQuestions error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExecuteHTTPFailureRetryAndRedaction(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		for _, retries := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/retries-%d", protocol, retries), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					call := calls.Add(1)
					if call == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = io.WriteString(w, `{"error":{"message":"temporary"}}`)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if protocol == ProtocolOpenAI {
						_, _ = io.WriteString(w, `{"answers":[],"model":"m","usage":{}}`)
					} else {
						_, _ = io.WriteString(w, `{"answers":{},"ok":true}`)
					}
				}))
				defer server.Close()
				base := server.URL
				if protocol == ProtocolOpenAI {
					base += "/v1"
				}
				_, err := Execute(context.Background(), Config{
					Protocol: protocol, BaseURL: base, APIKey: testAPIKey, MaxRetries: retries,
				}, Request{Model: "m", Questions: questionsFor(protocol)})
				wantCalls := int32(1 + retries)
				if calls.Load() != wantCalls {
					t.Fatalf("calls = %d, want %d", calls.Load(), wantCalls)
				}
				if retries == 0 {
					if err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "temporary") {
						t.Fatalf("error = %v, want status and body", err)
					}
				} else if err != nil {
					t.Fatalf("Execute after retry: %v", err)
				}
			})
		}
	}

	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		t.Run(protocol+"/unauthorized", func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprintf(w, `{"error":{"message":"rejected credential %s"}}`, testAPIKey)
			}))
			defer server.Close()
			base := server.URL
			if protocol == ProtocolOpenAI {
				base += "/v1"
			}
			_, err := Execute(context.Background(), Config{
				Protocol: protocol, BaseURL: base, APIKey: testAPIKey, MaxRetries: 2,
			}, Request{Model: "m", Questions: questionsFor(protocol)})
			if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "rejected credential [REDACTED]") {
				t.Fatalf("error = %v, want sanitized status and body detail", err)
			}
			if strings.Contains(err.Error(), testAPIKey) {
				t.Fatalf("error exposed API key: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("calls = %d, want no retry after 401", calls.Load())
			}
		})
	}
}

func TestExecuteRejectsEmptyAndInvalidSuccessBodies(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		for _, body := range []string{"", "{", "{\"model\":\"\xff\",\"answers\":[]}", `{"model":"m","answers":[]} trailing`, `{"model":"m","answers":[]} {"extra":true}`} {
			t.Run(protocol+fmt.Sprintf("/body-%q", body), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, body)
				}))
				defer server.Close()
				base := server.URL
				if protocol == ProtocolOpenAI {
					base += "/v1"
				}
				_, err := Execute(context.Background(), Config{Protocol: protocol, BaseURL: base, APIKey: testAPIKey},
					Request{Model: "m", Questions: questionsFor(protocol)})
				if err == nil {
					t.Fatal("Execute returned nil error for empty or invalid JSON response")
				}
			})
		}
	}
}

func TestExecuteCancellationStopsRetry(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		t.Run(protocol, func(t *testing.T) {
			firstResponse := make(chan struct{})
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"error":{"message":"retry me"}}`)
				close(firstResponse)
			}))
			defer server.Close()
			base := server.URL
			if protocol == ProtocolOpenAI {
				base += "/v1"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := Execute(ctx, Config{
					Protocol: protocol, BaseURL: base, APIKey: testAPIKey, MaxRetries: 2,
				}, Request{Model: "m", Questions: questionsFor(protocol)})
				done <- err
			}()
			select {
			case <-firstResponse:
				cancel()
			case <-time.After(5 * time.Second):
				t.Fatal("first request did not reach server")
			}
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
					t.Fatalf("Execute error = %v, want canceled", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Execute did not stop after cancellation")
			}
			if calls.Load() != 1 {
				t.Fatalf("calls = %d, want retry canceled", calls.Load())
			}
		})
	}
}

func TestExecuteRetryableTransportError(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		t.Run(protocol, func(t *testing.T) {
			var calls atomic.Int32
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					return nil, temporaryTestError{}
				}
				body := `{"answers":{},"ok":true}`
				if protocol == ProtocolOpenAI {
					body = `{"answers":[],"model":"m","usage":{}}`
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    r,
				}, nil
			})}
			base := "https://decision.test/prefix"
			if protocol == ProtocolOpenAI {
				base += "/v1"
			}
			_, err := Execute(context.Background(), Config{
				Protocol: protocol, BaseURL: base, APIKey: testAPIKey,
				HTTPClient: client, MaxRetries: 1,
			}, Request{Model: "m", Questions: questionsFor(protocol)})
			if err != nil {
				t.Fatalf("Execute after transport retry: %v", err)
			}
			if calls.Load() != 2 {
				t.Fatalf("calls = %d, want 2", calls.Load())
			}
		})
	}
}

func TestExecuteUsesProviderDefaultURLs(t *testing.T) {
	for _, protocol := range []string{ProtocolOpenAI, ProtocolOpenRouter} {
		t.Run(protocol, func(t *testing.T) {
			var gotURL string
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				gotURL = r.URL.String()
				body := `{"answers":{},"ok":true}`
				if protocol == ProtocolOpenAI {
					body = `{"answers":[],"model":"m","usage":{}}`
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    r,
				}, nil
			})}
			_, err := Execute(context.Background(), Config{
				Protocol: protocol, APIKey: testAPIKey, HTTPClient: client,
			}, Request{Model: "m", Questions: questionsFor(protocol)})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			want := openRouterDefaultBaseURL + "/decisions"
			if protocol == ProtocolOpenAI {
				want = openAIDefaultBaseURL + "/decisions"
			}
			if gotURL != want {
				t.Fatalf("request URL = %q, want %q", gotURL, want)
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type temporaryTestError struct{}

func (temporaryTestError) Error() string   { return "temporary network error" }
func (temporaryTestError) Timeout() bool   { return false }
func (temporaryTestError) Temporary() bool { return true }

func questionsFor(protocol string) json.RawMessage {
	if protocol == ProtocolOpenAI {
		return json.RawMessage(`[{"type":"predicate","instructions":"q"}]`)
	}
	return json.RawMessage(`{"q":{"type":"noul","instructions":"q"}}`)
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func TestRetryableStatusSet(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests, 500, 599} {
		if !retryableStatus(status) {
			t.Errorf("status %d should be retryable", status)
		}
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, 499} {
		if retryableStatus(status) {
			t.Errorf("status %d should not be retryable", status)
		}
	}
}
