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
