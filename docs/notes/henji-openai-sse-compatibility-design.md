# Henji OpenAI-Compatible SSE Compatibility Design

## Status

2026-10-08: migrated the common client from `github.com/openai/openai-go` v1.12.0
to `github.com/openai/openai-go/v3` v3.74.0 in the working tree.
Release and installed-binary deployment are pending.

## Problem

Henji sends every provider other than Anthropic and Google through its common
OpenAI-compatible streaming client. This includes OpenAI, OpenRouter, vMLX,
and the new MLX-LM endpoint.

MLX-LM emits a standard Server-Sent Events (SSE) comment while it is processing
a prompt:

```text
: keepalive 23/24

```

SSE clients must ignore comment-only events. The former v1.12.0 dependency
recognizes the comment line, but dispatches an event when it subsequently sees
the blank separator. Henji then attempts to decode the event's empty data as
JSON and fails with `unexpected end of JSON input` before MLX-LM's first model
chunk arrives.

This is a common-client robustness gap, not an MLX-LM-specific protocol.

## Goal

Accept valid SSE comment/heartbeat traffic without changing request formatting
or the interpretation of non-empty completion chunks.

## Non-goals

- Do not add an MLX-LM-only exception.
- Do not modify the MLX-LM server or suppress its keepalives.
- Do not change Anthropic or Google provider paths.
- Do not change model selection, authentication, JSON-schema handling, or
  retry policy.

## Implementation

The original v1 fix attached response middleware to the shared client. For
`text/event-stream` responses, it removed the blank delimiter of an SSE block
with no `data:` line before the SDK attempted JSON decoding.

SDK v3.74.0's SSE decoder already skips events with no data. The migration
removes that middleware and relies on the SDK decoder; comment-only events,
including CRLF keepalives, remain covered by the existing stream tests.

Non-empty completion chunks, API error JSON, and `[DONE]` continue through the
normal SDK stream handling.

## Why this is safe for the shared OpenAI layer

The change is confined to parsing an event that cannot represent a valid
OpenAI completion payload. It does not alter outgoing HTTP requests or decode
any non-empty JSON differently.

| Provider path | Expected effect |
| --- | --- |
| OpenAI | No behavior change; valid completion JSON continues unchanged. |
| OpenRouter and other OpenAI-compatible cloud APIs | No behavior change unless they send comment heartbeats, which will become supported. |
| vMLX | No behavior change for ordinary chunks. |
| MLX-LM | Fixes keepalive-related premature stream closure. |
| Anthropic / Google | Unaffected; they use separate clients. |

## Acceptance checks

1. Unit-test a stream containing a comment followed by a blank line and then a
   valid JSON completion chunk. The only yielded completion is the JSON chunk.
2. Unit-test a stream with multiple comments before its first completion.
3. Preserve parsing of `[DONE]` and API error JSON.
4. Run the existing OpenAI-compatible request tests unchanged.
5. Exercise a custom HTTP client through a local proxy and verify request
   fields, authentication, streamed text, and accumulated conversation history.
6. Exercise the Hayari and Shirushi CLI arguments against a local mock API:
   success emits only validated model JSON plus a newline; schema and API
   failures emit no stdout, exit 1, and keep human-readable stderr details.
   Preserve both token-limit fields when both are configured.
7. Run the same compatibility tests with the previous SDK through a Go overlay.
8. After installation, verify an invocation against the real MLX-LM server:

   ```sh
   henji -a mlxlm -m e4b "1+1は？答えだけで。"
   ```

## Rollout

1. Implement the dependency upgrade or narrowly scoped patch in the Henji
   repository.
2. Run the targeted tests and normal project test suite.
3. Install the rebuilt Henji binary.
4. Re-run the MLX-LM command above against `http://127.0.0.1:8081/v1`.
5. Keep the existing `localai` vMLX provider on port 8080 unchanged.

## SDK v3 migration compatibility

- Keep Chat Completions (`/chat/completions`); Responses and decision-model
  support are separate future work.
- `ChatCompletionAccumulator` remains available in v3.74.0. Preserve the
  existing accumulation and provider-independent saved-conversation format.
- Keep request fields, schema dialect selection, model selection, and retry
  policy. Both SDK versions default to two internal HTTP retries; Henji's
  configured retries and schema retries remain separate.
- Preserve v1's handling of server retry hints: `Retry-After-Ms` takes
  precedence over `Retry-After`, which accepts seconds or an HTTP date. Honor
  the first parseable delay only when it is nonnegative and less than 60
  seconds. Otherwise remove both hints before the SDK's retry decision so it
  uses its ordinary short backoff. This prevents v3's default wait of up to
  120 seconds (or its refusal to retry above that limit) from changing the
  caller's behavior. `WithMaxRetryDelay` alone would stop retries at its limit
  rather than restore the old backoff.
- SDK v3's `Error()` reports only an HTTP status summary. Henji wraps API
  errors with its previous format: method, quoted URL, status, and provider
  error JSON. This preserves stderr and explicit `--output json` diagnostics;
  status/code/message classification still uses the original SDK error.
- Google retains its existing use of the SDK error type for shared error
  classification. Its HTTP implementation is unchanged.
- Azure-specific authentication was already removed before this migration
  (see `fix-roadmap.md`, “Azure OpenAI / Azure AD サポートの廃止”). No
  Azure-specific path is introduced here.

Local mock tests cover request formatting, images, streaming, keepalives,
`[DONE]`, API errors, schema validation, conversation continuation, and proxy
use. Retry tests include the 60-second boundary, millisecond hints and their
precedence, HTTP dates, and delays exceeding v3's limit. Hayari-equivalent CLI
tests recover from an initial 429/60-second hint or 500/121-second hint within
the existing test subprocess deadline, preserving raw JSON output and the
original request payload. These mock tests do not establish real-provider
availability or model quality.

## Live OpenRouter verification (2026-10-08)

After the user registered an OpenRouter key and authorized three CLI runs,
the corrected working tree was built into an isolated test binary. All three
runs used `google/gemini-2.5-flash-lite`, the actual caller prompts and schemas,
stdin input, `-q`, `--no-cache`, and `--json-schema-retries 0`, without
`--output json`.

| Caller-equivalent check | Result | Time | stdout bytes |
| --- | --- | --- | --- |
| Hayari: English title translation | Valid `translated` with Japanese title | 1.021 s | 92 |
| Hayari: French title | Valid `skipped`, no `title` key | 0.723 s | 26 |
| Shirushi: fictional library document | Valid Japanese `summary` | 1.232 s | 406 |

Every run exited 0 with empty stderr. stdout contained one UTF-8 model JSON
object with a trailing newline. The title met Hayari's nonempty/500-character
limit; the summary met Shirushi's nonempty/400-character/1–5-line limits, with
no empty lines. Outputs also stayed within the callers' 4,096/16,384-byte
limits and 30/120-second deadlines. Unknown keys, duplicate keys, and trailing
JSON were checked.

Hayari's configuration retained `max-completion-tokens: 100` alongside
`--max-tokens 512`; Shirushi used `--max-tokens 1024` without a completion-token
setting. Both profiles succeeded with those settings. This does not identify
which token field OpenRouter or its upstream provider gives precedence to.

The three runs used synthetic input and dedicated settings/data directories;
the installed binary and normal configuration were unchanged. Only these three
CLI runs were started. SDK/Henji HTTP retries retained their existing defaults;
individual HTTP attempt counts were not instrumented. Real 429/5xx retry-hint
behavior and live error paths remain untested; the regression tests cover those
cases locally.

Temporary evidence: `/tmp/henji-openrouter-live-kpj1_r3j/results.json` and the
three `case-*.stdout`/`case-*.stderr` files. The binary SHA256 is recorded in
that report. The key is stored outside the repository and is not included in
the report or this note.
