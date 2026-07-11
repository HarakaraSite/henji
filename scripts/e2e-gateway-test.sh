#!/usr/bin/env bash
# E2E test against a real local OpenAI-compatible gateway.
#
# This is NOT run in CI: it depends on a local gateway (e.g. mlx-lm, Ollama,
# LM Studio) running on the developer's machine, so it can't be automated
# on a Forgejo runner. Run it manually after touching --output json,
# request building (stream.go), or provider request code.
#
# Usage:
#   GATEWAY_URL=http://localhost:8080/v1 MODEL=mlx-community/gemma-4-E2B-it-qat-4bit ./scripts/e2e-gateway-test.sh
#
# Defaults match the author's local mlx-lm gateway setup.

set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080/v1}"
MODEL="${MODEL:-mlx-community/gemma-4-E2B-it-qat-4bit}"

if ! command -v jq >/dev/null 2>&1; then
	echo "jq is required to run this script" >&2
	exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$(mktemp -d)/henji"
CONFIG_HOME="$(mktemp -d)"

cleanup() {
	rm -rf "$(dirname "$BIN")" "$CONFIG_HOME"
}
trap cleanup EXIT

echo "== building henji =="
(cd "$REPO_ROOT" && go build -o "$BIN" .)

mkdir -p "$CONFIG_HOME/henji"
cat >"$CONFIG_HOME/henji/henji.yml" <<EOF
max-input-chars: 50000
temp: 1.0
topp: 1.0
apis:
  gateway:
    base-url: ${GATEWAY_URL}
    api-key: dummy
    models:
      ${MODEL}:
        max-input-chars: 50000
        aliases: []
      nonexistent-model-xyz:
        max-input-chars: 50000
        aliases: []
default-api: gateway
default-model: ${MODEL}
EOF

run() {
	XDG_CONFIG_HOME="$CONFIG_HOME" "$BIN" --no-cache "$@"
}

fail() {
	echo "FAIL: $1" >&2
	exit 1
}

echo "== checking gateway reachability (${GATEWAY_URL}) =="
if ! curl -s --max-time 3 "${GATEWAY_URL}/models" >/dev/null; then
	fail "gateway at ${GATEWAY_URL} is not reachable; start it before running this script"
fi

echo "== test 1: --output json success path (short prompt) =="
out="$(echo "日本の首都はどこですか？一言で答えてください。" | run --output json)"
echo "$out"
echo "$out" | jq -e '.version == 1' >/dev/null || fail "version field missing/wrong"
echo "$out" | jq -e '.content[0].type == "text"' >/dev/null || fail "content[0].type != text"
echo "$out" | jq -e '.content[0].text | length > 0' >/dev/null || fail "content text is empty (max-input-chars truncation regression?)"
echo "$out" | jq -e '.error == null' >/dev/null || fail "unexpected error field on success"
echo "PASS"

echo "== test 2: --output json success path (longer prompt via arg) =="
out="$(run --output json "Goについて2文で説明して")"
echo "$out"
echo "$out" | jq -e '.content[0].text | length > 10' >/dev/null || fail "response text suspiciously short"
echo "PASS"

echo "== test 3: --output json error path (model not found) =="
out="$(run --output json --model nonexistent-model-xyz "hi" || true)"
echo "$out"
echo "$out" | jq -e '.version == 1' >/dev/null || fail "version field missing/wrong"
echo "$out" | jq -e '.error.message | length > 0' >/dev/null || fail "error.message missing/empty"
echo "$out" | jq -e '.content == null' >/dev/null || fail "unexpected content on error"
echo "PASS"

echo "== test 4: no-truncation regression check (max-input-chars unset) =="
cat >"$CONFIG_HOME/henji/henji.yml" <<EOF
apis:
  gateway:
    base-url: ${GATEWAY_URL}
    api-key: dummy
    models:
      ${MODEL}:
        aliases: []
default-api: gateway
default-model: ${MODEL}
temp: 1.0
topp: 1.0
EOF
out="$(echo "日本の首都はどこですか？一言で答えてください。" | run --output json)"
echo "$out"
echo "$out" | jq -e '.content[0].text | length > 0' >/dev/null || fail "prompt was truncated to empty (PR#18 regression)"
echo "PASS"

echo "== test 5: --file attaches UTF-8 text input =="
attachment="$CONFIG_HOME/attachment.txt"
attachment_token="HENJI_FILE_E2E_TOKEN_4829"
printf 'Attachment token: %s\n' "$attachment_token" >"$attachment"
out="$(run --output json -f "$attachment" "Reply with the attachment token exactly.")"
echo "$out"
echo "$out" | jq -e --arg token "$attachment_token" '.content[0].text | contains($token)' >/dev/null || fail "file content was not reflected in the response"
echo "PASS"

echo
echo "All e2e checks passed."
