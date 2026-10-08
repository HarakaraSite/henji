# Release checkpoints

This document defines henji's pre-tag reference checks, the environment they
require, and the recorded result of each release. The portable release gate that
runs on the Forgejo runner is defined by `.forgejo/release-profile.yml` and
rendered into `.forgejo/workflows/release.yml`.

## Gate boundary

| Gate | Where it runs | Checks |
|---|---|---|
| Portable | Forgejo Actions runner | `go test ./...`, `go vet ./...`, CGO-free cross builds, `SHA256SUMS` |
| Reference | developer workstation, before tagging | `go test -race ./...`, `./scripts/e2e-openrouter-test.sh` |

Reference checks are not run in CI: the runner has no C toolchain for `-race`
and no funded OpenRouter key, and the E2E check makes real, billed requests.

## Reference environment

- Go toolchain matching `go.mod`.
- A C toolchain (gcc or clang) with `CGO_ENABLED=1` so `go test -race ./...`
  can build.
- Network access and a funded `OPENROUTER_API_KEY` for
  `scripts/e2e-openrouter-test.sh`, which drives the OpenAI-compatible path
  against `https://openrouter.ai/api/v1` using `deepseek/deepseek-v4.1-flash`
  (override with `MODEL`).

## Reference checks

| Name | Command | Requires |
|---|---|---|
| race | `go test -race ./...` | C toolchain |
| openrouter-e2e | `./scripts/e2e-openrouter-test.sh` | network + `OPENROUTER_API_KEY` |

Run both before creating a release tag, and record the outcome below.

## Workflow linting

There is no Forgejo-native Actions linter. `actionlint` is the de facto tool,
but it does not know Forgejo's `forgejo.*` context and therefore reports six
`undefined variable "forgejo"` errors on the release workflow; every other check
is clean. To lint locally while keeping the `forgejo.*` expressions the release
profile requires:

```sh
actionlint -ignore 'undefined variable "forgejo"' .forgejo/workflows/release.yml
```

## Results

### v2.1.10 — 2026-10-08

- Source changes: conversation persistence/locking, JSON error handling and
  UTF-8 truncation, updated Anthropic/SQLite/golden dependencies, maintained
  YAML v3 module, and OpenAI Go SDK v3 with legacy CLI/error/retry behavior.
- race: passed `go test -race ./...` with Go 1.26.0 and `CGO_ENABLED=1`.
  GCC 14.2 and its development dependencies were extracted into a temporary
  directory and used through a compiler wrapper; no system packages were
  installed and no CI package-installation steps were added.
- openrouter-e2e: passed all five checks in `scripts/e2e-openrouter-test.sh`
  with `MODEL=google/gemini-2.5-flash-lite` and Go 1.26.0: short/long JSON
  success, invalid-model JSON error, unset-input-limit regression, and text
  attachment. Settings and data were isolated; the user authorized the billed
  checks. The invalid model returned HTTP 400 with the expected provider
  details preserved in stderr and the JSON error envelope.
- Caller compatibility: three additional real OpenRouter runs passed for
  Hayari title translation/skipping and Shirushi summarization. Raw model
  JSON, exit 0, empty stderr, caller limits and both configured Hayari token
  fields were verified; see `notes/henji-openai-sse-compatibility-design.md`.
- Profile review used `apply-forgejo-go-release-profile`: version 2, mandatory
  checksums, native logs, five CGO-free targets, automatic Forgejo contexts,
  and the current workflow remain appropriate. Rendering a temporary candidate
  introduced only comments and equivalent ldflags placement, so neither the
  profile nor the workflow needs regeneration.
- Forgejo environment: version `16.0.3+gitea-1.22.0`; native logs for the
  previous release's run 170/job 191 returned HTTP 200 through both run ZIP
  and job text endpoints. Existing workflow and rendered candidate both passed
  actionlint with only the documented `forgejo.*` exclusion.
- Portable gate: Go 1.26.0 normal tests and vet passed locally. Forgejo
  [Actions run 30](https://forge.harakara.site/littleisland/henji/actions/runs/30)
  (API run 172/job 193) passed test, vet, all five builds, checksums and upload.
- [Release v2.1.10](https://forge.harakara.site/littleisland/henji/releases/tag/v2.1.10)
  was published from `09081ee0d8ae8ff05c95cd255f39e40fb7e4fb33` with five
  binaries and `SHA256SUMS`, `draft=false`, `prerelease=false`. The downloaded
  Linux/amd64 binary reports `henji version v2.1.10`; its SHA-256 matches the
  published checksum list:
  `2302a0618ad5d2c03756ee17bacff2d0835a0ad2db9b40e4d196be6ba98ccf2a`.

### v2.1.9 — 2026-09-13

- Portable gate: `go build ./...`, `go vet ./...`, and `go test ./...` passed
  locally; Forgejo Actions run 29 passed (test, vet, five target builds).
- Release: five assets published; `henji-linux-amd64 --version` reports
  `v2.1.9` (SHA-256 `cf4c3d71a4dc3e15d0fb7f0c60a55d49d282b471f783fbae7973278627e67413`).
- race: **TODO — not executed** in the verification environment (no C toolchain).
- openrouter-e2e: passed all five checks (JSON success short/long, error path,
  no-truncation regression, `--text` attachment) with
  `deepseek/deepseek-v4.1-flash`. This replaced the former local-gateway script.
- Anthropic (not a profile gate): reached the API with a valid request and
  returned `400 credit balance is too low` (authentication and request building
  verified, no successful completion).
