package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const openAIDecisionQuestions = `[{"name":"relevant","type":"predicate","instructions":"Is it relevant?"},{"name":"urgent","type":"predicate","instructions":"Is it urgent?"}]`
const openRouterDecisionQuestions = `{"relevant":{"type":"noul","instructions":"Is it relevant?"},"urgent":{"type":"noul","instructions":"Is it urgent?"}}`

func TestDecisionCommandDiscovery(t *testing.T) {
	ensureFlagsInitialized()
	for _, test := range []struct {
		args []string
		want bool
	}{
		{[]string{"decision", "-m", "test", "--questions", "q.json"}, true},
		{[]string{"-a", "openrouter", "decision", "-m", "test"}, true},
		{[]string{"--output", "json", "decision", "--bad"}, true},
		{[]string{"__complete", "decision", "--questions", ""}, true},
		{[]string{"__completeNoDesc", "decision", ""}, true},
		{[]string{"decision explain this"}, false},
		{[]string{"--", "decision"}, false},
		{[]string{"--model", "decision", "explain"}, false},
		{[]string{"explain", "decision"}, false},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			args := append([]string{"henji"}, test.args...)
			require.Equal(t, test.want, isDecisionCmd(args))
			if test.want {
				require.False(t, needsConversationDB(args))
			}
		})
	}
}

func TestResolveDecisionAPI(t *testing.T) {
	cfg := Config{API: "gateway", APIs: APIs{{Name: "gateway", BaseURL: "https://chat.invalid/v1", DecisionProtocol: "openrouter", DecisionBaseURL: "https://decision.invalid/proxy/alpha", Models: map[string]Model{"vision-id": {Aliases: []string{"vision"}, Vision: true}}}}}
	api, model, err := resolveDecisionAPI(&cfg, "", "vision")
	require.NoError(t, err)
	require.Equal(t, "https://decision.invalid/proxy/alpha", api.DecisionBaseURL)
	require.Equal(t, "vision-id", model.Name)
	require.True(t, model.Vision)
	_, model, err = resolveDecisionAPI(&cfg, "", "new-model-not-in-catalog")
	require.NoError(t, err)
	require.Equal(t, "new-model-not-in-catalog", model.Name)
	for name, base := range map[string]string{"openai": "https://api.openai.com/v1", "openrouter": "https://openrouter.ai/api/alpha"} {
		api, _, err := resolveDecisionAPI(&Config{}, name, "test")
		require.NoError(t, err)
		require.Equal(t, name, api.DecisionProtocol)
		require.Equal(t, base, api.DecisionBaseURL)
	}
	_, _, err = resolveDecisionAPI(&cfg, "missing", "test")
	require.ErrorContains(t, err, "not configured")
	_, _, err = resolveDecisionAPI(&Config{APIs: APIs{{Name: "gateway"}}}, "gateway", "test")
	require.ErrorContains(t, err, "decision-protocol")
}

func writeDecisionQuestions(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "questions.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestCLIDecisionNativeJSONWithoutConversationStorage(t *testing.T) {
	const input = "  原文の空白と改行を保持する。\nsecond line\n"
	for _, protocol := range []string{"openai", "openrouter"} {
		t.Run(protocol, func(t *testing.T) {
			questions, response := openAIDecisionQuestions, ` {"model":"test-model","answers":[{"name":"relevant","type":"refusal","reason":"test refusal"}],"usage":{"input_tokens":3},"future":{"number":92233720368547758071}} `
			if protocol == "openrouter" {
				questions, response = openRouterDecisionQuestions, ` {"model":"test-model","answers":{"relevant":{"type":"noul","noul":0.75}},"usage":{"cost":0.001},"provider":"test-provider","future":{"number":92233720368547758071}} `
			}
			requests := make(chan map[string]json.RawMessage, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/gateway/alpha/decisions", r.URL.Path)
				require.Equal(t, "Bearer fake", r.Header.Get("Authorization"))
				var body map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			configText := fmt.Sprintf("default-api: gateway\ndefault-model: translation-model\noutput: json\nmax-retries: 0\nmax-input-chars: 1\nmax-tokens: 512\nmax-completion-tokens: 100\njson-schema: /missing/generation-schema.json\napis:\n  gateway:\n    decision-protocol: %s\n    decision-base-url: %s/gateway/alpha/\n    base-url: %s/chat-only\n    api-key: fake\n", protocol, server.URL, server.URL)
			env, cachePath := cliTestEnvironment(t, configText)
			// A file at XDG_DATA_HOME makes any attempted cache creation fail,
			// even when the test user can bypass Unix directory permissions.
			require.NoError(t, os.WriteFile(filepath.Dir(cachePath), []byte("not a directory"), 0o600))
			env = append(env, "HENJI_OUTPUT=json")
			cmd := cliTestCommand(t, env, "decision", "-m", "test-model", "--questions", writeDecisionQuestions(t, questions))
			cmd.Stdin = strings.NewReader(input)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			require.NoError(t, cmd.Run(), "%s", stderr.String())
			require.Equal(t, strings.TrimSpace(response), strings.TrimSpace(stdout.String()))
			require.True(t, strings.HasSuffix(stdout.String(), "\n"))
			require.Empty(t, stderr.String())
			require.NoDirExists(t, cachePath)
			body := <-requests
			require.Len(t, body, 3)
			require.JSONEq(t, questions, string(body["questions"]))
			inputKey := "input"
			if protocol == "openrouter" {
				inputKey = "state"
			}
			var sentInput string
			require.NoError(t, json.Unmarshal(body[inputKey], &sentInput))
			require.Equal(t, input, sentInput)
		})
	}
}

func TestCLIDecisionImageAndModelAlias(t *testing.T) {
	imageData := []byte("\x89PNG\r\n\x1a\nimage test bytes")
	imagePath := filepath.Join(t.TempDir(), "photo.bin")
	require.NoError(t, os.WriteFile(imagePath, imageData, 0o600))
	for _, protocol := range []string{"openai", "openrouter"} {
		t.Run(protocol, func(t *testing.T) {
			questions := openAIDecisionQuestions
			if protocol == "openrouter" {
				questions = openRouterDecisionQuestions
			}
			requests := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				if protocol == "openrouter" {
					fmt.Fprint(w, `{"model":"vision-id","answers":{}}`)
				} else {
					fmt.Fprint(w, `{"model":"vision-id","answers":[]}`)
				}
			}))
			defer server.Close()
			env, cachePath := cliTestEnvironment(t, fmt.Sprintf("apis:\n  gateway:\n    decision-protocol: %s\n    decision-base-url: %s\n    api-key: fake\n    models:\n      vision-id:\n        aliases: [vision]\n        vision: true\n", protocol, server.URL))
			cmd := cliTestCommand(t, env, "decision", "-a", "gateway", "-m", "vision", "--questions", writeDecisionQuestions(t, questions), "--image", imagePath)
			cmd.Stdin = strings.NewReader("inspect this image\n")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			require.NoError(t, cmd.Run(), "%s", stderr.String())
			require.True(t, json.Valid(stdout.Bytes()))
			require.NoDirExists(t, cachePath)
			body := string(<-requests)
			require.Contains(t, body, `"model":"vision-id"`)
			require.Contains(t, body, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageData))
			require.Contains(t, body, "inspect this image")
			if protocol == "openrouter" {
				require.Contains(t, body, `"type":"image_url"`)
				require.NotContains(t, body, `"input_image"`)
			} else {
				require.Contains(t, body, `"type":"input_image"`)
			}
		})
	}
}

func TestCLIDecisionFailuresLeaveStdoutEmpty(t *testing.T) {
	for _, protocol := range []string{"openai", "openrouter"} {
		t.Run(protocol, func(t *testing.T) {
			questions := openAIDecisionQuestions
			if protocol == "openrouter" {
				questions = openRouterDecisionQuestions
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":{"message":"test authentication failure"}}`)
			}))
			defer server.Close()
			cfg := fmt.Sprintf("output: json\nmax-retries: 0\napis:\n  gateway:\n    decision-protocol: %s\n    decision-base-url: %s\n    api-key: fake\n", protocol, server.URL)
			questionsPath := writeDecisionQuestions(t, questions)
			for _, test := range []struct {
				name, input, configText, diagnostic string
				args                                []string
				calls                               int32
			}{
				{"API failure", "input", cfg, "test authentication failure", []string{"-a", "gateway", "-m", "test", "--questions", questionsPath}, 1},
				{"model required", "input", cfg, "model", []string{"--questions", questionsPath}, 0},
				{"questions required", "input", cfg, "questions", []string{"-m", "test"}, 0},
				{"invalid questions", "input", cfg, "questions", []string{"-a", "gateway", "-m", "test", "--questions", writeDecisionQuestions(t, `{"broken":`)}, 0},
				{"unknown flag", "input", cfg, "unexpected", []string{"--unexpected"}, 0},
				{"empty input", "", cfg, "requires text", []string{"-a", "gateway", "-m", "test", "--questions", questionsPath}, 0},
				{"invalid utf8", string([]byte{0xff}), cfg, "UTF-8", []string{"-a", "gateway", "-m", "test", "--questions", questionsPath}, 0},
				{"config failure", "input", "output: json\napis: [invalid", "configuration", []string{"-m", "test", "--questions", questionsPath}, 0},
				{"duplicate questions", "input", cfg, "only once", []string{"-m", "test", "--questions", questionsPath, "--questions", questionsPath}, 0},
			} {
				t.Run(test.name, func(t *testing.T) {
					env, cachePath := cliTestEnvironment(t, test.configText)
					env = append(env, "HENJI_OUTPUT=json")
					cmd := cliTestCommand(t, env, append([]string{"decision"}, test.args...)...)
					cmd.Stdin = strings.NewReader(test.input)
					var stdout, stderr bytes.Buffer
					cmd.Stdout, cmd.Stderr = &stdout, &stderr
					before := requests.Load()
					err := cmd.Run()
					var exitErr *exec.ExitError
					require.ErrorAs(t, err, &exitErr)
					require.Equal(t, 1, exitErr.ExitCode())
					require.Empty(t, stdout.String())
					require.Contains(t, stderr.String(), test.diagnostic)
					require.Equal(t, test.calls, requests.Load()-before)
					require.NoDirExists(t, cachePath)
				})
			}
		})
	}
}

func TestCLIDecisionHelpAndCompletion(t *testing.T) {
	for _, args := range [][]string{{"decision", "--help"}, {"__complete", "decision", "--quest"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env, cachePath := cliTestEnvironment(t, "output: json\n")
			cmd := cliTestCommand(t, env, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			require.NoError(t, cmd.Run(), "%s", stderr.String())
			require.Contains(t, stdout.String(), "questions")
			require.NotContains(t, stdout.String(), "\x1b[")
			require.NoDirExists(t, cachePath)
		})
	}
}

func TestCLIDecisionHTTPProxy(t *testing.T) {
	for _, protocol := range []string{"openai", "openrouter"} {
		t.Run(protocol, func(t *testing.T) {
			questions := openAIDecisionQuestions
			if protocol == "openrouter" {
				questions = openRouterDecisionQuestions
			}
			var calls atomic.Int32
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "http://decision.invalid/proxy-prefix/decisions", r.RequestURI)
				require.Equal(t, "Bearer fake", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				if protocol == "openai" {
					fmt.Fprint(w, `{"model":"test","answers":[]}`)
				} else {
					fmt.Fprint(w, `{"model":"test","answers":{}}`)
				}
			}))
			defer proxy.Close()
			env, _ := cliTestEnvironment(t, fmt.Sprintf("http-proxy: %s\napis:\n  gateway:\n    api-key: fake\n    decision-protocol: %s\n    decision-base-url: http://decision.invalid/proxy-prefix\n", proxy.URL, protocol))
			cmd := cliTestCommand(t, env, "decision", "-a", "gateway", "-m", "test", "--questions", writeDecisionQuestions(t, questions))
			cmd.Stdin = strings.NewReader("input")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			require.NoError(t, cmd.Run(), "%s", stderr.String())
			require.Equal(t, int32(1), calls.Load())
			require.True(t, json.Valid(stdout.Bytes()))
		})
	}
}

func TestCLIDecisionKeyCommandPriority(t *testing.T) {
	for _, mode := range []string{"success", "fail"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "Bearer fake-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"model":"test","answers":{}}`)
			}))
			defer server.Close()
			keyCommand := fmt.Sprintf("%q -test.run=^TestAPIKeyCommandHelper$", os.Args[0])
			env, _ := cliTestEnvironment(t, fmt.Sprintf("output: json\napis:\n  gateway:\n    api-key-cmd: '%s'\n    api-key: fallback-must-not-be-used\n    decision-protocol: openrouter\n    decision-base-url: %s\n", keyCommand, server.URL))
			env = append(env, "HENJI_TEST_API_KEY_COMMAND="+mode)
			cmd := cliTestCommand(t, env, "decision", "-a", "gateway", "-m", "test", "--questions", writeDecisionQuestions(t, openRouterDecisionQuestions))
			cmd.Stdin = strings.NewReader("input")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if mode == "success" {
				require.NoError(t, err, "%s", stderr.String())
				require.True(t, json.Valid(stdout.Bytes()))
				require.Equal(t, int32(1), calls.Load())
				require.Empty(t, stderr.String())
			} else {
				require.Error(t, err)
				require.Empty(t, stdout.String())
				require.Contains(t, stderr.String(), "Cannot exec api-key-cmd")
				require.Equal(t, int32(0), calls.Load())
			}
		})
	}
}

func TestCLIDecisionCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sending os.Interrupt to a subprocess is not supported on Windows")
	}
	for _, protocol := range []string{"openai", "openrouter"} {
		t.Run(protocol, func(t *testing.T) {
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
			}))
			defer server.Close()
			questions := openAIDecisionQuestions
			if protocol == "openrouter" {
				questions = openRouterDecisionQuestions
			}
			env, cachePath := cliTestEnvironment(t, fmt.Sprintf("output: json\napis:\n  gateway:\n    api-key: fake\n    decision-protocol: %s\n    decision-base-url: %s\n", protocol, server.URL))
			cmd := cliTestCommand(t, env, "decision", "-a", "gateway", "-m", "test", "--questions", writeDecisionQuestions(t, questions))
			cmd.Stdin = strings.NewReader("input")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			require.NoError(t, cmd.Start())
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("decision did not reach the mock API")
			}
			require.NoError(t, cmd.Process.Signal(os.Interrupt))
			var exitErr *exec.ExitError
			require.ErrorAs(t, cmd.Wait(), &exitErr)
			require.Equal(t, 1, exitErr.ExitCode())
			require.Empty(t, stdout.String())
			require.Regexp(t, "canceled|interrupt", stderr.String())
			require.NoDirExists(t, cachePath)
		})
	}
}
