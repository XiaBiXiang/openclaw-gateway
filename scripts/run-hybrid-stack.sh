#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MLX_LOG="${MLX_SERVER_LOG_PATH:-$ROOT_DIR/mlx-server.log}"
MLX_PORT="${MLX_SERVER_PORT:-8000}"
MLX_HOST="${MLX_SERVER_HOST:-127.0.0.1}"
MLX_MODEL_REF="${1:-${MLX_MODEL:-${MODEL_PATH:-}}}"
MLX_PID=""

cleanup() {
  if [ -n "${MLX_PID}" ] && kill -0 "${MLX_PID}" 2>/dev/null; then
    kill "${MLX_PID}" 2>/dev/null || true
    wait "${MLX_PID}" 2>/dev/null || true
  fi
}

trap cleanup EXIT INT TERM

if ! lsof -nP -iTCP:${MLX_PORT} -sTCP:LISTEN >/dev/null 2>&1; then
  if [ -z "$MLX_MODEL_REF" ]; then
    echo "MLX model is required. Pass it as the first argument or set MLX_MODEL." >&2
    exit 1
  fi

  "$ROOT_DIR/scripts/run-mlx-server.sh" "$MLX_MODEL_REF" >"${MLX_LOG}" 2>&1 &
  MLX_PID=$!
fi

for _ in $(seq 1 60); do
  if curl -fsS "http://${MLX_HOST}:${MLX_PORT}/v1/models" >/dev/null 2>&1; then
    exec "$ROOT_DIR/scripts/run-local.sh"
  fi
  sleep 1
done

echo "mlx server did not become ready on ${MLX_HOST}:${MLX_PORT}" >&2
exit 1
