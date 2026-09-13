# Release checkpoints

This document defines henji's pre-tag reference checks, the environment they
require, and the recorded result of each release. The portable release gate that
runs on the Forgejo runner is defined by `.forgejo/release-profile.yml` and
rendered into `.forgejo/workflows/release.yml`.

## Gate boundary

| Gate | Where it runs | Checks |
|---|---|---|
| Portable | Forgejo Actions runner | `go test ./...`, `go vet ./...`, CGO-free cross builds, `SHA256SUMS` |
| Reference | developer workstation, before tagging | `go test -race ./...`, `./scripts/e2e-gateway-test.sh` |

Reference checks are not run in CI: the runner has no local model gateway, and
`-race` needs a C toolchain that the CGO-free release build deliberately avoids.

## Reference environment

- Go toolchain matching `go.mod`.
- A C toolchain (gcc or clang) with `CGO_ENABLED=1` so `go test -race ./...`
  can build.
- A local OpenAI-compatible gateway (mlx-lm, Ollama, or LM Studio) reachable at
  `GATEWAY_URL`, with a configured model id, for `scripts/e2e-gateway-test.sh`.

## Reference checks

| Name | Command | Requires |
|---|---|---|
| race | `go test -race ./...` | C toolchain |
| gateway-e2e | `./scripts/e2e-gateway-test.sh` | local OpenAI-compatible gateway |

Run both before creating a release tag, and record the outcome below.

## Results

### v2.1.9 — 2026-09-13

- Portable gate: `go build ./...`, `go vet ./...`, and `go test ./...` passed
  locally; Forgejo Actions run 29 passed (test, vet, five target builds).
- Release: five assets published; `henji-linux-amd64 --version` reports
  `v2.1.9` (SHA-256 `cf4c3d71a4dc3e15d0fb7f0c60a55d49d282b471f783fbae7973278627e67413`).
- race: **TODO — not executed** in the verification environment (no C toolchain).
- gateway-e2e: **TODO — not executed** (no local gateway available).
- Additional real-provider check (not a profile gate): OpenRouter over the
  OpenAI-compatible path passed JSON success, `--text` attachment, and the error
  path. Anthropic reached the API with a valid request and returned
  `400 credit balance is too low` (authentication and request building verified,
  no successful completion).
