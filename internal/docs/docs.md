henji is an LLM client for the command line, built for pipelines. This
manual is task-oriented and lists the pitfalls behind each task. For the
one-line flag reference, run `henji -h`.

## Invocation basics

The prompt is assembled from arguments, an optional file, and stdin, in that
order:

    henji "explain this error"                # args only
    cat error.log | henji                     # stdin only
    cat error.log | henji "what went wrong?"  # args first, then stdin,
                                              # joined by a blank line
    henji -f report.txt "summarize this"       # args first, then file content

Piped input is indented before it is appended, so it remains visually distinct
from the instruction supplied as arguments.

`-f` / `--file` accepts one UTF-8 text file. A second `--file` is an error,
and binary-looking files are rejected rather than silently being mangled. For
multiple files, concatenate them with the shell and pipe the result to stdin.

Output contract for model invocations:

- **stdout carries only the model's response** (or the JSON envelope with
  `--output json`). It is safe to pipe or capture.
- A progress spinner, "Conversation saved" notices, and error details go to
  **stderr**. `-q` silences non-error stderr chatter.
- When stdout is not a terminal, the response is plain text with no ANSI
  codes. Markdown rendering only happens on a TTY (`-r` disables it there too).
- Exit status is `0` on success and non-zero on failure.
- With `--output json`, errors reached during model execution also produce an
  error envelope on stdout. Startup errors such as an invalid flag, config, or
  schema file are reported on stderr before a JSON session exists.
- Prompts must be supplied as arguments and/or stdin. With no prompt, henji
  exits with an error instead of waiting for interactive input.
- `Ctrl-C` cancels a model request or an upstream command that has not closed
  stdin yet; it does not wait for that command to reach EOF.

## Choosing a provider and model

Models and providers ("APIs") are defined in the config file. To see what is
configured:

    henji --list-models
    henji --list-models --output json   # {"version":1,"apis":[{"name":...,
                                        #  "base_url":...,"models":[{"id":...,"aliases":[...]}]}]}

`base_url` (present when an API entry sets `base-url`) tells local gateways
apart from cloud endpoints in scripts, without hardcoding API names.

- `-m <model>` accepts a model ID or alias from the config.
- `-a <api>` selects the provider. The configured `default-model` belongs to
  one API, so pair `-a` with `-m`; otherwise the selected API may not contain
  the default model.
- A model entry may declare `fallback:`. henji retries with it when the API
  returns 404 for the selected model.
- API entries named `anthropic` and `google` use native protocols. Every other
  API entry uses the OpenAI-compatible protocol.

Local gateways (Ollama, mlx-lm, LM Studio, llama.cpp server, ...):

- Use the OpenAI-compatible endpoint and include `/v1` in `base-url`
  (for example, `http://localhost:11434/v1` for Ollama).
- henji always supplies an API key on this path. Common local servers do not
  require one; use a placeholder when your server does not validate keys.

## Getting machine-readable output

There are three levels, from loosest to strictest:

1. `--format --format-as json` asks the model for JSON but does not validate it.
   Avoid this alone for scripts that require a guaranteed shape.
2. `--output json` wraps the model response in a reliable one-line envelope:

       # success
       {"version":1,"conversation_id":"<sha1>","content":[{"type":"text","text":"..."}],"model":"..."}

       # model-execution failure (exit status 1; partial content may be present)
       {"version":1,"error":{"code":"error","message":"..."}}

       git diff | henji --output json "suggest a commit message" | jq -r '.content[0].text'

3. `--json-schema <file>` sends a JSON Schema through the provider's native
   structured-output mechanism, then validates the answer client-side before
   printing. On failure, henji sends the model a correction message quoting
   the validation error and asking it to respond again with ONLY JSON that
   strictly matches the schema, then retries (`--json-schema-retries`,
   default 2). The failed attempt stays in conversation history for context;
   if every retry repeats the same mistake, the schema itself may be
   ambiguous or too strict for that model.

       henji --json-schema review.json "review this diff" < diff.patch | jq '.findings[]'

   `--json-schema` and `--output json` compose: the validated JSON document is
   delivered as a string inside the envelope's `content[0].text`.

Structured-output pitfalls:

- Google's schema dialect is an OpenAPI 3.0 subset. It rejects
  `additionalProperties`; keep Google-targeted schemas flat and simple.
- henji sends `strict:true` only for an API entry named `openai`. Other
  OpenAI-compatible entries receive the schema without `strict`; client-side
  validation still catches invalid responses.
- OpenAI's strict mode requires every object in the schema to set
  `"additionalProperties": false`; a schema without it is rejected by the
  API before generation even starts.
- Small local models may wrap JSON in Markdown fences or ignore the schema.
  Repeated violations exhaust the retries and exit non-zero. Adding "raw JSON
  only, no code fences" to the prompt can help weaker models comply.
- Live output is suppressed while validating. The response is printed once,
  only after it passes validation.

## A typical agent loop

    # 1. Discover providers and models.
    henji --list-models --output json

    # 2. Run a task and capture its conversation ID.
    id=$(git diff | henji --output json "review this diff" | jq -r .conversation_id)

    # 3. Follow up in the same conversation.
    henji --output json -c "$id" "now suggest fixes for finding 1"

## Conversations

Successful model conversations are saved automatically (metadata in SQLite,
message bodies on disk) unless `--no-cache` is set.

- `-l` prints a tab-separated list of saved conversations with their last save
  time in local time (`YYYY-MM-DD HH:MM:SS TZ`); `-t <title>` names one at save
  time. It never opens an interactive selector.
- `-C` continues the most recent conversation; `-c <id-or-title>` continues a
  specific one. IDs may be abbreviated to a unique SHA-1 prefix.
- `-s <id-or-title>` prints a saved conversation without calling a model.
- `-d <id-or-title>` deletes a conversation.

Two equally valid ways to name a conversation for continuation:

- Title-based: `-t <title>` at save time, then `-c <title>` to continue it.
- ID-based: capture `conversation_id` from `--output json` (see "Getting
  machine-readable output" and "A typical agent loop"), then `-c <id>`.
  Convenient when scripting, since the ID is already in hand.

    henji --list
    henji --show <id-or-title>
    henji --continue <id-or-title> "follow-up prompt"

Pitfalls:

- Continuing a conversation resends its entire history. Per-request input and
  cumulative cost grow as the conversation gets longer. Start fresh when prior
  context is not needed.
- Continuing with `-c`/`-C` and no `-a`/`-m` flags restores the API and model
  that were used when the conversation was saved; there's no need to repeat
  them to keep talking to the same provider.
- If `--max-tokens` is too low, the answer may stop mid-sentence without a
  warning. Raise it rather than repeatedly recovering the rest with `-C`.
- Reasoning models spend hidden thinking tokens first. If their per-model
  `max-completion-tokens` config is too small, the visible answer can be empty.

## Roles

Define named system prompts under `roles:` in the config file:

    roles:
      shell:
        - you are a shell expert
        - you only output one-liners, no explanation

Use one with `-R shell`; list them with `--list-roles`. Each role line may also
be an `http(s)://` or `file://` URL whose contents become system prompt text.

## Configuration and tuning

- Config: `$XDG_CONFIG_HOME/henji/henji.yml`, default
  `~/.config/henji/henji.yml`.
- Conversation data: `$XDG_DATA_HOME/henji/`, default
  `~/.local/share/henji/`.
- Scalar settings with an environment mapping can be overridden with a
  `HENJI_` prefix, for example `HENJI_TEMP=0.2` or
  `HENJI_MAX_TOKENS=4000`. Structured sections such as `apis` and `roles`
  remain YAML configuration.

Some tuning knobs intentionally have no flag and are config/environment-only:
`temp`, `topp`, `topk`, `stop`, `max-retries`, `word-wrap`, and `http-proxy`.

`max-input-chars` (global or per-model) truncates the combined prompt before it
is sent; the tail is dropped silently. `--no-limit` disables this truncation.

API keys are resolved in this order, highest priority first:

1. `api-key-cmd`: stdout of a directly executed command. It does not run
   through a shell, so `$USER` and `$(...)` are not expanded.
2. `api-key-env`: a named environment variable.
3. `api-key`: plaintext in the mode-0600 config file.
4. Provider default environment variable (`OPENAI_API_KEY`,
   `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, ...).
