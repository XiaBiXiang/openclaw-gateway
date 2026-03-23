#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_CONFIG="$ROOT_DIR/configs/config.local.json"
OPENCLAW_CONFIG="${OPENCLAW_CONFIG:-${HOME}/.openclaw/openclaw.json}"
OPENCLAW_PROVIDER_ID="${OPENCLAW_PROVIDER_ID:-}"

if [ ! -f "$DEFAULT_CONFIG" ]; then
  DEFAULT_CONFIG="$ROOT_DIR/configs/config.local.example.json"
fi

CONFIG_PATH="${1:-$DEFAULT_CONFIG}"

if [ ! -f "$CONFIG_PATH" ]; then
  echo "config file not found: $CONFIG_PATH" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for scripts/run-local.sh" >&2
  exit 1
fi

API_KEY_ENV="$(jq -r '.providers.cloud.api_key_env // empty' "$CONFIG_PATH")"

if [ -n "$API_KEY_ENV" ] && [ -z "${!API_KEY_ENV:-}" ] && [ -n "$OPENCLAW_PROVIDER_ID" ]; then
  if [ ! -f "$OPENCLAW_CONFIG" ]; then
    echo "openclaw config not found: $OPENCLAW_CONFIG" >&2
    exit 1
  fi

  API_KEY_VALUE="$(jq -r --arg provider "$OPENCLAW_PROVIDER_ID" '.models.providers[$provider].apiKey // empty' "$OPENCLAW_CONFIG")"
  if [ -z "$API_KEY_VALUE" ]; then
    echo "could not derive ${API_KEY_ENV} from provider ${OPENCLAW_PROVIDER_ID} in $OPENCLAW_CONFIG" >&2
    exit 1
  fi

  export "${API_KEY_ENV}=${API_KEY_VALUE}"
fi

cd "$ROOT_DIR"

if [ -x "$ROOT_DIR/bin/openclaw-gateway" ]; then
  exec "$ROOT_DIR/bin/openclaw-gateway" -config "$CONFIG_PATH"
fi

exec go run ./cmd/gateway -config "$CONFIG_PATH"
