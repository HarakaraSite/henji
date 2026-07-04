# henji (mods fork)

AI for the command line, built for pipelines.

This is an actively maintained fork of [charmbracelet/mods](https://github.com/charmbracelet/mods),
which was archived on March 9, 2026. The fork focuses on local LLM usage and MCP integration.

## What Changed from Upstream

### Bug fixes

| Area | Fix |
|---|---|
| OpenAI | Panic when response has no choices |
| Ollama | Channel leak / deadlock on stream cancellation |
| Google | nil panic and response body leak on request failure |
| All providers | `cancelRequest` goroutine leak replaced with `defer cancel` |
| All providers | `max-completion-tokens` (o1 models) now correctly wired through to the API |
| All providers | `api-key-env` priority over `api-key-cmd` restored to match documented order |
| MCP | Connection caching across tool-call rounds; `errgroup` cancels siblings on first failure |

### Security

- `henji.yml` is created with `0600` permissions (was `0644`)
- Google API key moved from URL query parameter to `x-goog-api-key` header, preventing key exposure in transport error messages
- `henji.yml` and `*.bak` added to `.gitignore`

### Dependencies

All dependencies updated to current versions, including security patches for `x/net` and `x/crypto`.

### Removals

- `Config.System` field removed (was unused)
- Native Ollama client removed; Ollama is served by the OpenAI-compatible path (`base-url: http://localhost:11434/v1`)
- Rarely-used flags removed to keep `--help` and the code small:
  - `--ask-model`, `--show-last`, `--delete-older-than`, `--dirs`, `--reset-settings`, `--theme` (features removed)
  - `-P`/`--prompt`, `-p`/`--prompt-args` prompt-echo modes (features removed)
  - `--fanciness`, `--status-text` spinner tuning (fixed defaults now)
  - `--temp`, `--topp`, `--topk`, `--stop`, `--max-retries`, `--word-wrap`, `--http-proxy` (still configurable via `henji.yml` / `HENJI_*` env)

## Installation

### Build from source

```sh
git clone <this-repo>
cd henji
go build -o henji .
```

> `go install` support will be available once the module is published.

### Shell completions

```bash
henji completion bash -h
henji completion zsh -h
henji completion fish -h
henji completion powershell -h
```

## Recommended Setup

> For more setup patterns and copy-pasteable examples (adding new providers,
> keychain-based API keys, `--output json` for scripting/agents, reasoning
> model quirks), see the [cookbook](docs/cookbook.md).

### Local LLM (Ollama / mlx-lm)

Point henji at your local OpenAI-compatible endpoint. Local servers don't check the key, but henji's OpenAI-compatible request path always sends one, so set `api-key` to any placeholder value:

```yaml
# ~/.config/henji/henji.yml
apis:
  local:
    base-url: http://localhost:11434/v1  # Ollama default
    api-key: local
    models:
      llama3.2:
        aliases: ["llama"]
        max-input-chars: 32000

default-model: llama3.2
```

```sh
echo "explain this error" | henji
ls -la | henji summarize these files
```

### MCP tool call limit

When using MCP servers, set a tool-call limit to prevent runaway loops. `0` means unlimited (default).

```yaml
# ~/.config/henji/henji.yml
max-tool-calls: 10
```

### API key management

Preferred order (most secure first):

1. **`api-key-cmd`** — shell command whose stdout is the key; keys never touch disk

   ```yaml
   api-key-cmd: op read "op://vault/openai/key"
   # or:
   api-key-cmd: rbw get -f OPENAI_API_KEY chat.openai.com
   ```

   The command must write only the key to stdout. If it fails or exits
   non-zero, henji reports an error rather than silently falling back to a
   lower-priority source.

2. **`api-key-env`** — read from a named environment variable

   ```yaml
   api-key-env: OPENAI_API_KEY
   ```

3. **`api-key`** — plaintext in `henji.yml` (stored at `0600`; avoid committing to version control)

4. **Default env** — provider-specific fallback (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.)

### Environment variable overrides

Any `henji.yml` setting can be overridden with a `HENJI_` prefixed environment variable:

```sh
export HENJI_DEFAULT_MODEL=llama3.2
export HENJI_MAX_TOKENS=2000
export HENJI_FORMAT=true
```

## Usage

```sh
# Read the full task-oriented manual
henji docs

# Pipe command output to an LLM
ls -la | henji "explain these files"
cat error.log | henji "what went wrong?"

# Prompt only
henji "write a haiku about Go"

# Continue a conversation
henji -C "now make it funnier"
```

For more worked examples, see [`examples.md`](examples.md)
([日本語](examples.ja.md)) and [`features.md`](features.md)
([日本語](features.ja.md)). See also the [cookbook](docs/cookbook.md) above
for provider setup and scripting/agent patterns.

### Flags

| Flag | Description |
|---|---|
| `-m`, `--model` | Specify the model to use |
| `-a`, `--api` | OpenAI compatible REST API to use (openai, localai, anthropic, ...) |
| `-f`, `--format` | Ask the LLM to format the response (e.g. markdown, json) |
| `--format-as` | Specify output format (used with `--format`) |
| `--json-schema` | Path to a JSON Schema file; constrains and validates the response (see [Structured Output](#structured-output)) |
| `--json-schema-retries` | Times to ask the model to correct a response that fails schema validation (default 2) |
| `-e`, `--editor` | Edit the prompt in `$EDITOR` (only when no other args and stdin is a TTY) |
| `-q`, `--quiet` | Only output errors to stderr |
| `-r`, `--raw` | Print raw response without syntax highlighting |
| `-R`, `--role` | Specify a custom role (system prompt) |
| `--list-roles` | List roles defined in your configuration file |
| `--max-tokens` | Maximum tokens in response |
| `--max-tool-calls` | Maximum agentic tool call rounds; `0` = unlimited |
| `--no-limit` | Do not limit response tokens |
| `--settings` | Open settings file in `$EDITOR` |
| `-h`, `--help` | Show help and exit |
| `-v`, `--version` | Show version and exit |

Tuning knobs that rarely change between runs — sampling parameters (`temp`,
`topp`, `topk`, `stop`), `max-retries`, `word-wrap`, and `http-proxy` — have
no dedicated flags; set them in `henji.yml` or override per run with the
corresponding `HENJI_*` environment variable (e.g. `HENJI_TEMP=0.2`).

#### Conversations

| Flag | Description |
|---|---|
| `-t`, `--title` | Set conversation title |
| `-l`, `--list` | List saved conversations |
| `-c`, `--continue` | Continue a conversation by title or SHA-1 |
| `-C`, `--continue-last` | Continue the last conversation |
| `-s`, `--show` | Show a saved conversation |
| `-d`, `--delete` | Delete conversations by title or SHA-1 |
| `--no-cache` | Do not save this conversation |

#### MCP

| Flag | Description |
|---|---|
| `--mcp-list` | List configured MCP servers |
| `--mcp-list-tools` | List available tools from enabled MCP servers |
| `--mcp-disable` | Disable specific MCP servers for this run |

## Custom Roles

Roles set a system prompt for a session. Define them in `henji.yml`:

```yaml
roles:
  shell:
    - you are a shell expert
    - you do not explain anything
    - you simply output one liners to solve the problems you're asked
    - you do not provide any explanation whatsoever, ONLY the command
```

```sh
henji --role shell list files sorted by size
```

## Cloud Providers

### OpenAI

```sh
export OPENAI_API_KEY=sk-...
```

### Anthropic

```sh
export ANTHROPIC_API_KEY=sk-ant-...
```

### Google Gemini

```sh
export GOOGLE_API_KEY=...
```

### Groq

```sh
export GROQ_API_KEY=...
```

### Azure OpenAI

```sh
export AZURE_OPENAI_KEY=...
```

Configure the `base-url` and `azure-deployment` in `henji.yml` as well.

## Structured Output

`--format json` only asks the model to *try* to respond as JSON; it doesn't
guarantee the response actually matches any particular shape. `--json-schema`
is stricter: it passes your schema to the provider's native structured-output
feature (Anthropic's `output_config.format`, the OpenAI-compatible
`response_format.json_schema` used by OpenAI/Groq/local gateways, or
Google's `generationConfig.responseSchema`) and additionally validates the
response against the schema client-side before printing it.

```sh
henji --json-schema review-schema.json "review this diff for security issues" < diff.patch
```

If the response fails validation, henji tells the model what was wrong and
asks it to try again (up to `--json-schema-retries` times, default 2) instead
of silently resending the same prompt.

Notes:

- Real OpenAI enforces `strict: true` reliably; other OpenAI-compatible
  dialects (Groq models outside the `gpt-oss-*` family, local gateways like
  Ollama/mlx-lm, Azure) get the schema without `strict`, since they may
  reject or ignore it — the client-side validation step still catches
  anything that slips through.
- Google's schema support is an OpenAPI 3.0 subset and doesn't accept every
  JSON Schema keyword. Confirmed via a real request: `additionalProperties`
  is rejected outright (`400 Unknown name "additionalProperties"`); `$ref`/
  `oneOf`-heavy schemas are also likely to be rejected. Keep schemas simple,
  and drop `additionalProperties` when targeting the Google dialect.
- `--json-schema` suppresses live streaming output — since a failed response
  gets discarded and retried, henji only prints once the answer has actually
  passed validation.

## MCP Integration

MCP (Model Context Protocol) allows the LLM to call external tools defined by MCP servers.

```yaml
# ~/.config/henji/henji.yml
mcp-servers:
  filesystem:
    type: stdio
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

max-tool-calls: 10  # recommended; 0 = unlimited
```

```sh
henji --mcp-list-tools          # inspect available tools
henji --mcp-disable filesystem  # disable a server for this run
```

## License

[MIT](LICENSE) — original work by [Charmbracelet, Inc.](https://charm.sh)
