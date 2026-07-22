# Henji OpenAI-Compatible SSE Compatibility Design

## Status

Implemented in the working tree; release and installed-binary deployment are
pending.

## Problem

Henji sends every provider other than Anthropic and Google through its common
OpenAI-compatible streaming client. This includes OpenAI, OpenRouter, vMLX,
and the new MLX-LM endpoint.

MLX-LM emits a standard Server-Sent Events (SSE) comment while it is processing
a prompt:

```text
: keepalive 23/24

```

SSE clients must ignore comment-only events. The installed OpenAI Go dependency
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

## Proposed approach

Keep `github.com/openai/openai-go v1.12.0` and attach a small response
middleware to the shared OpenAI-compatible client. For `text/event-stream`
responses, it removes only the blank delimiter of an SSE block that contains
no `data:` line. The v1 decoder therefore never receives an event created only
by comments.

The rule is protocol-based:

```text
Ignore the delimiter of an SSE block with no `data:` line.
```

It applies before JSON decoding. Non-empty data, including API error objects,
completion chunks, and `[DONE]`, retain the existing behavior.

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
4. Run existing OpenAI, OpenRouter, and vMLX request tests unchanged.
5. Add an MLX-LM integration test that streams a response with a keepalive.
6. Verify an installed Henji invocation succeeds:

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

## Future: OpenAI Go v3 migration

Henji will remain on `github.com/openai/openai-go v1.12.0` for this fix. The
comment-only SSE compatibility filter is intentionally a narrow v1 workaround;
it does not require a dependency major-version migration.

As of 2026-07-22, the current upstream module is
`github.com/openai/openai-go/v3 v3.44.0`. Moving to it is future work for a
separate Henji major-release decision, not a prerequisite for MLX-LM support.
Refresh the upstream version, migration guide, and support status when that
work is actually scheduled; no v1 end-of-support commitment has been assumed
here.

The v3 migration must include these explicit decisions and checks:

- Migrate the OpenAI request/stream types and replace the removed
  `ChatCompletionAccumulator` behavior used for saved conversations.
- Decide whether to retain Azure and Azure AD. Their v1 SDK helpers are absent
  from v3; if they remain supported, implement and test the required endpoint
  and authentication handling. Do not remove them before that product decision.
- Re-run real compatibility checks for OpenAI, OpenRouter, and configured
  local OpenAI-compatible providers, including streaming, `[DONE]`, API error
  payloads, images, JSON Schema, conversation continuation, and HTTP proxy
  use.
