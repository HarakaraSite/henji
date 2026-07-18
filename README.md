# henji (mods fork)

AI for the command line, built for pipelines.

*henji* is named after the Japanese word for “reply” (返事).

This is an actively maintained fork of [charmbracelet/mods](https://github.com/charmbracelet/mods),
which was archived on March 9, 2026. The fork focuses on local LLM usage and
treating henji as a Unix filter: `stdin → LLM → stdout`, composable with the
rest of your shell pipeline rather than acting on your behalf. henji does not
open an input form or a model picker: provide a prompt as arguments, `--text`,
and/or stdin, and select a configured model with `--model` / `--api` when
needed.

A public mirror is available on [Codeberg](https://codeberg.org/littleisland/henji).

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

### Security

- `henji.yml` is created with `0600` permissions (was `0644`); an existing file with looser permissions (e.g. inherited from a pre-v2 install) is restricted to `0600` automatically before it's read, and refused outright if it's owned by another user (Unix only; not yet enforced on Windows)
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
  - `--fanciness`, `--status-text` spinner tuning (a fixed, minimal spinner is
    shown on stderr while a TTY request is in progress)
  - `--temp`, `--topp`, `--topk`, `--stop`, `--max-retries`, `--word-wrap`, `--http-proxy` (still configurable via `henji.yml` / `HENJI_*` env)
- **MCP (Model Context Protocol) support removed entirely**, along with
  `--mcp-list`, `--mcp-list-tools`, `--mcp-disable`, `--max-tool-calls`, and
  the `mcp-servers`/`mcp-timeout` config keys. MCP let a model call external
  tools with no per-tool approval or read/write distinction, which a security
  review flagged as a real risk when henji processes untrusted content
  (a webpage, log, or issue containing text aimed at the model rather than
  the user). Rather than build the allowlisting/sandboxing/audit machinery a
  safe agentic tool loop needs, this fork returns henji to a plain Unix
  filter: file and network access stay in the hands of `cat`, `curl`, `find`,
  and the rest of the shell pipeline around henji. See "Generate, review,
  then run" below for the intended pattern when a task needs those.

## Installation

### Build from source

```sh
git clone https://forge.harakara.site/littleisland/henji.git
cd henji
go build -o henji .
```

> This repository is not yet publicly accessible, so `go install` isn't
> available; building from a local clone is the only supported path for now.

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

default-api: local
default-model: llama3.2
```

```sh
echo "explain this error" | henji
ls -la | henji summarize these files
```

### API key management

Preferred order (most secure first):

1. **`api-key-cmd`** — directly executed command whose stdout is the key; keys never touch disk

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

Scalar `henji.yml` settings that have an environment mapping can be overridden
with a `HENJI_` prefixed environment variable:

```sh
export HENJI_MODEL=llama3.2
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

# Choose a configured API/model explicitly
henji --api local --model llama3.2 "explain this error"

# Attach one text file while keeping stdin available for a pipeline
henji --text report.txt "summarize this report"

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
| `-m`, `--model` | Select a configured model ID or alias |
| `-a`, `--api` | Select a configured API endpoint; pair it with `--model` |
| `--text` | Attach one UTF-8 text file to the prompt |
| `--image` | Attach one JPEG, PNG, or WebP image to the prompt (max 3 MiB; requires `vision: true`) |
| `--format` | Ask the LLM to format the response (e.g. markdown, json) |
| `--format-as` | Specify output format (used with `--format`) |
| `--json-schema` | Path to a JSON Schema file; constrains and validates the response (see [Structured Output](#structured-output)) |
| `--json-schema-retries` | Times to ask the model to correct a response that fails schema validation (default 2) |
| `--output` | Output format: `text` or `json` (single-line JSON envelope for scripting/agents; see the [cookbook](docs/cookbook.md#--output-json-for-scripting-and-ai-agents)) |
| `-q`, `--quiet` | Hide the stderr spinner and non-error status messages |
| `-r`, `--raw` | Print raw text instead of Markdown rendering on a TTY |
| `-R`, `--role` | Specify a custom role (system prompt) |
| `--list-roles` | List roles defined in your configuration file |
| `--list-models` | List configured APIs and their models (respects `--output json`; see the [cookbook](docs/cookbook.md#discovering-whats-configured)) |
| `--max-tokens` | Maximum tokens in response |
| `--no-limit` | Do not limit response tokens |
| `-h`, `--help` | Show help and exit |
| `-v`, `--version` | Show version and exit |

Tuning knobs that rarely change between runs — sampling parameters (`temp`,
`topp`, `topk`, `stop`), `max-retries`, `word-wrap`, and `http-proxy` — have
no dedicated flags; set them in `henji.yml` or override per run with the
corresponding `HENJI_*` environment variable (e.g. `HENJI_TEMP=0.2`).

`--text` accepts exactly one UTF-8 text file up to 3 MiB. `--image` accepts
exactly one JPEG, PNG, or WebP image up to 3 MiB and requires `vision: true`
on the selected model. The attachment limit remains in force with `--no-limit`.
henji combines inputs in this order: prompt arguments, text, image, then stdin.
It rejects binary or invalid UTF-8 text files; use stdin for deliberate
multi-file concatenation:

```yaml
# henji.yml: enable image input only for a model you have verified supports it
apis:
  local:
    models:
      vision-model:
        vision: true
```

`--text` and `--image` attachments are not stored in saved conversations.
Attach the file or image again when a continued conversation needs it.

```sh
cat chapter-*.txt | henji "compare these chapters"
```

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

`--list` always prints a tab-separated list; it does not open a selection UI.
Use the displayed ID or title with `--show`, `--continue`, or `--delete`:

```sh
henji --list
henji --show a1b2c3d
henji --continue a1b2c3d "now suggest fixes"
```

For bulk cleanup, use SQLite only to select IDs and let `henji --delete`
remove both the database row and the matching conversation-body cache file:

```sh
DB="$HOME/.local/share/henji/conversations/henji.db"

# Example: delete conversations older than a chosen date, including their bodies.
sqlite3 "$DB" \
  "SELECT id FROM conversations WHERE updated_at < '2026-01-01';" |
while IFS= read -r id; do
  henji --delete "$id"
done
```

If `XDG_DATA_HOME` or `cache-path` is set, derive `DB` from that location
instead. Do not delete SQLite rows alone: the corresponding `<id>.gob` file
would remain on disk.

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

For Groq (and other OpenAI-compatible providers whose API entry is not named
`openai`), reference that variable explicitly with `api-key-env: GROQ_API_KEY`
in the provider's API entry.

### Azure OpenAI

```sh
export AZURE_OPENAI_KEY=...
```

Configure an `azure` or `azure-ad` API entry with its `base-url` and model
entries in `henji.yml` as well.

## Structured Output

`--format --format-as json` only asks the model to *try* to respond as JSON; it doesn't
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

## Generate, review, then run

henji doesn't call tools or touch your filesystem on its own — it reads
stdin and writes stdout, nothing else. When a task needs file access, a
network request, or a shell command, ask henji to generate that command and
run it yourself:

```sh
henji -R shell "find the 10 largest files under the current directory"
```

Read what came back before running it — `less`/`bat`, not `cat`, so long or
adversarial output doesn't scroll straight past you — and never pipe
henji's output directly into a shell:

```sh
# Don't: skips the review step entirely, executes whatever henji wrote
henji -R shell "..." | sh

# Do: look at it first, then run it yourself
henji -R shell "..." > candidate.sh
less candidate.sh
bash candidate.sh
```

This matters most when the prompt or piped-in content comes from somewhere
you don't fully trust (a fetched webpage, a log file, an issue body): text
aimed at the model rather than at you can end up in its answer, so treat
generated commands the same way you'd treat a shell snippet copied from a
random webpage.

## Verification

Run the unit and provider mock tests before changing behavior:

```sh
go test ./...
go vet ./...
```

For a real OpenAI-compatible gateway (mlx-lm, Ollama, or LM Studio), run the
manual E2E check. It validates JSON success and error paths, the
`max-input-chars` regression, and `--text` attachment; it is intentionally
not part of CI because a Forgejo runner has no local model gateway.

```sh
GATEWAY_URL=http://localhost:8080/v1 \
MODEL=<configured-model-id> \
./scripts/e2e-gateway-test.sh
```

## License

[MIT](LICENSE) — original work by [Charmbracelet, Inc.](https://charm.sh)
