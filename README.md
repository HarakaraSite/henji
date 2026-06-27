# Mods (fork)

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

- `henji.yml` and `.bak` are created with `0600` permissions (was `0644`)
- Google API key moved from URL query parameter to `x-goog-api-key` header, preventing key exposure in transport error messages
- `henji.yml` and `*.bak` added to `.gitignore`

### Dependencies

All dependencies updated to current versions, including security patches for `x/net` and `x/crypto`.

### Removals

- `Config.System` field removed (was unused)

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

### Local LLM (Ollama / mlx-lm)

The simplest setup requires no API key. Point Mods at your local OpenAI-compatible endpoint:

```yaml
# ~/.config/henji/henji.yml
apis:
  local:
    base-url: http://localhost:11434/v1  # Ollama default
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

   The command must write only the key to stdout.

2. **`api-key-env`** — read from a named environment variable

   ```yaml
   api-key-env: OPENAI_API_KEY
   ```

3. **`api-key`** — plaintext in `henji.yml` (stored at `0600`; avoid committing to version control)

4. **Default env** — provider-specific fallback (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.)

## Usage

```sh
# Pipe command output to an LLM
ls -la | henji "explain these files"
cat error.log | henji "what went wrong?"

# Prompt only
henji "write a haiku about Go"

# Continue a conversation
henji -C "now make it funnier"
```

### Flags

| Flag | Description |
|---|---|
| `-m`, `--model` | Specify the model to use |
| `-M`, `--ask-model` | Choose model interactively |
| `-a`, `--api` | OpenAI compatible REST API to use (openai, localai, anthropic, ...) |
| `-f`, `--format` | Ask the LLM to format the response (e.g. markdown, json) |
| `--format-as` | Specify output format (used with `--format`) |
| `-P`, `--prompt` | Include prompt from args and stdin; truncate stdin to N lines |
| `-p`, `--prompt-args` | Include prompt from args in the response |
| `-e`, `--editor` | Edit the prompt in `$EDITOR` (only when no other args and stdin is a TTY) |
| `-q`, `--quiet` | Only output errors to stderr |
| `-r`, `--raw` | Print raw response without syntax highlighting |
| `-R`, `--role` | Specify a custom role (system prompt) |
| `--list-roles` | List roles defined in your configuration file |
| `-x`, `--http-proxy` | Use HTTP proxy for API connections |
| `--max-retries` | Maximum number of retries |
| `--max-tokens` | Maximum tokens in response |
| `--max-tool-calls` | Maximum agentic tool call rounds; `0` = unlimited |
| `--no-limit` | Do not limit response tokens |
| `--stop` | Up to 4 sequences where the API stops generating |
| `--word-wrap` | Wrap output at width (default 80) |
| `--status-text` | Text shown while generating |
| `--settings` | Open settings file in `$EDITOR` |
| `--reset-settings` | Restore settings to default |
| `--dirs` | Print the directories where henji stores its data |
| `--theme` | UI theme: `charm`, `catppuccin`, `dracula`, `base16` |
| `-h`, `--help` | Show help and exit |
| `-v`, `--version` | Show version and exit |

#### Conversations

| Flag | Description |
|---|---|
| `-t`, `--title` | Set conversation title |
| `-l`, `--list` | List saved conversations |
| `-c`, `--continue` | Continue a conversation by title or SHA-1 |
| `-C`, `--continue-last` | Continue the last conversation |
| `-s`, `--show` | Show a saved conversation |
| `-S`, `--show-last` | Show the previous conversation |
| `-d`, `--delete` | Delete conversations by title or SHA-1 |
| `--delete-older-than` | Delete conversations older than duration (`10d`, `1mo`, `1y`) |
| `--no-cache` | Do not save this conversation |

#### MCP

| Flag | Description |
|---|---|
| `--mcp-list` | List configured MCP servers |
| `--mcp-list-tools` | List available tools from enabled MCP servers |
| `--mcp-disable` | Disable specific MCP servers for this run |

#### Advanced

| Flag | Description |
|---|---|
| `--temp` | Sampling temperature (0.0–2.0, -1.0 to disable) |
| `--topp` | Top-P (0.0–1.0, -1.0 to disable) |
| `--topk` | Top-K (-1 to disable) |
| `--fanciness` | Level of fanciness |

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
