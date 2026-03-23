#!/usr/bin/env bash

set -euo pipefail

MODEL_REF="${1:-${MLX_MODEL:-${MODEL_PATH:-}}}"
HOST="${MLX_SERVER_HOST:-127.0.0.1}"
PORT="${MLX_SERVER_PORT:-8000}"
LOG_LEVEL="${MLX_SERVER_LOG_LEVEL:-INFO}"
PROMPT_CACHE_SIZE="${MLX_PROMPT_CACHE_SIZE:-32768}"
MAX_TOKENS="${MLX_MAX_TOKENS:-1024}"

if [ -z "$MODEL_REF" ]; then
  echo "usage: $0 <model-path-or-repo-id>" >&2
  echo "set MLX_MODEL or pass a local path / Hugging Face repo id such as mlx-community/Qwen3-4B-Instruct-2507-4bit" >&2
  exit 1
fi

case "$MODEL_REF" in
  /*|./*|../*)
    if [ ! -d "$MODEL_REF" ]; then
      echo "model path not found: $MODEL_REF" >&2
      exit 1
    fi
    ;;
esac

exec conda run -n mlx-lm python -m mlx_lm.server \
  --model "$MODEL_REF" \
  --host "$HOST" \
  --port "$PORT" \
  --log-level "$LOG_LEVEL" \
  --prompt-cache-size "$PROMPT_CACHE_SIZE" \
  --max-tokens "$MAX_TOKENS"
