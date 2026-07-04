# henji Cookbook

Practical, copy-pasteable patterns for setting up and using henji day to day.
For the full flag reference see `henji -h`; for the design rationale behind
`--output json` see [`docs/notes/json-output-plan.md`](notes/json-output-plan.md).

## Where things live

- Settings: `$XDG_CONFIG_HOME/henji/henji.yml`, or `~/.config/henji/henji.yml`
  if unset (macOS and Linux share this location; Windows uses `%LOCALAPPDATA%`)
- Conversation history/cache: `$XDG_DATA_HOME/henji/`, or
  `~/.local/share/henji/` if unset

## Setting up a new provider

Every provider is just an entry under `apis:` in `henji.yml`. The pattern is
always the same: `base-url`, a way to get the API key, and a `models:` map.

### Cloud provider (Anthropic, Google, OpenRouter, ...)

Store the key in the macOS Keychain instead of plaintext in the config file:

```sh
security add-generic-password -a "$(whoami)" -s "MY-PROVIDER-API" -w "the actual key"
```

Then reference it with `api-key-cmd`. Note `api-key-cmd` is parsed and executed
directly (not through a shell), so **`$USER`/`$(whoami)` won't expand** —
write the literal username:

```yaml
apis:
  anthropic:
    base-url: https://api.anthropic.com/v1
    api-key-cmd: security find-generic-password -a masat -s ANTHROPIC-API -w
    models:
      claude-sonnet-5:
        aliases: ["sonnet-5", "sonnet"]
        max-input-chars: 4000000
```

(You can also omit `-a` entirely and match by service name only:
`security find-generic-password -s ANTHROPIC-API -w`.)

### Local gateway (Ollama, mlx-lm, LM Studio, ...)

Local servers don't check the key, but henji's OpenAI-compatible request path
always sends one — set `api-key` to any placeholder value, and make sure
`base-url` includes `/v1`:

```yaml
apis:
  local:
    base-url: http://localhost:8080/v1
    api-key: local
    models:
      mlx-community/gemma-4-E2B-it-qat-4bit:
        aliases: ["local", "gemma"]
        max-input-chars: 50000
```

### Finding current model names

Provider model catalogs change constantly — a model name from six months ago
may 404 today. Query the provider's own models endpoint rather than guessing:

```sh
curl -s https://api.anthropic.com/v1/models \
  -H "x-api-key: $(security find-generic-password -a masat -s ANTHROPIC-API -w)" \
  -H "anthropic-version: 2023-06-01" | jq -r '.data[].id'
```

Some providers publish a floating "latest" alias that always points at their
current flagship, so you don't have to update `henji.yml` every release
(verify what it currently resolves to via the response's `model`/
`modelVersion` field, since these do change over time):

```yaml
gemini-flash-latest:
  aliases: ["flash"]
```

## Discovering what's configured

```sh
henji --list-models                 # human-readable
henji --list-models --output json   # machine-readable, for scripts/agents
```

## Switching your default provider temporarily

`default-api`/`default-model` at the top of `henji.yml` control what runs when
you omit `--api`/`--model`. Just edit the values in place — no need to move
anything:

```yaml
default-api: openrouter
default-model: z-ai/glm-5.2   # the real model id, not an alias
```

## Reasoning models and `max-completion-tokens`

Reasoning models (DeepSeek R1/V4, Kimi, GLM, o1/o3, etc.) spend tokens on a
hidden "thinking" pass before emitting any visible text. If
`max-completion-tokens` is set too low (henji's shipped template default is a
conservative `100`), the entire budget can be consumed by reasoning before any
content is produced — the response comes back **empty, with no error**.

Fix: give reasoning models their own generous budget at the model level:

```yaml
apis:
  openrouter:
    models:
      moonshotai/kimi-latest:
        max-completion-tokens: 4096   # overrides the global 100
```

## Feeding large files without spending your own tokens

If you're an AI agent (or scripting) driving henji, pipe file contents
through the shell instead of reading the file into your own context first —
the shell does the concatenation, not you:

```sh
cat large_file.txt | henji --output json "summarize this"
cat file1.txt file2.txt | henji --output json "diff these"
henji --output json "summarize" < large_file.txt
```

`henji` combines the CLI argument (instruction) and stdin (content) as
`instruction + "\n\n" + stdin`, so the split between "what to do" and "the
data to do it to" is exactly this: instruction as an argument, bulk content
via stdin.

## `--output json` for scripting and AI agents

```sh
henji --output json "explain this error" < error.log
```

Success:

```json
{"version":1,"content":[{"type":"text","text":"..."}],"model":"...","conversation_id":"..."}
```

Failure (exit code 1):

```json
{"version":1,"error":{"code":"error","message":"..."}}
```

`jq` recipes:

```sh
henji --output json "..." | jq -r '.content[0].text'   # extract just the text
henji --output json "..." | jq -e '.error == null'      # assert success
```

## Structured output with `--json-schema`

`--format json` only asks nicely; `--json-schema` uses the provider's native
structured-output feature and validates the response client-side, so
downstream tooling can trust the shape without defensive parsing.

Say you want a security review of a diff, reduced to a fixed shape a script
can consume:

```json
// review-schema.json
{
  "type": "object",
  "properties": {
    "summary": { "type": "string" },
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "severity": { "type": "string", "enum": ["low", "medium", "high", "critical"] },
          "file": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["severity", "file", "description"]
      }
    }
  },
  "required": ["summary", "findings"]
}
```

```sh
git diff main | henji --json-schema review-schema.json \
  "review this diff for security issues" | jq '.findings[] | select(.severity == "critical")'
```

If the model's first answer doesn't validate, henji shows it the validation
error and asks it to correct itself, up to `--json-schema-retries` times
(default 2), before giving up with a clear error instead of handing your
script malformed JSON.

Combine it with `--output json` to get the structured answer inside the
usual scripting envelope:

```sh
henji --output json --json-schema review-schema.json "..." | jq -r '.content[0].text | fromjson'
```

Targeting `--api google`? Drop `additionalProperties` from the schema —
confirmed via a real request that Gemini rejects it outright (`400 Unknown
name "additionalProperties"`). See README.md's Structured Output notes for
other Google schema limitations.
