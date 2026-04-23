#!/usr/bin/env bash
set -euo pipefail

# ── Colors ──────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { printf "${CYAN}  ➜${RESET} %s\n" "$*"; }
ok()    { printf "${GREEN}  ✔${RESET} %s\n" "$*"; }
warn()  { printf "${YELLOW}  ⚠${RESET} %s\n" "$*"; }
fail()  { printf "${RED}  ✖${RESET} %s\n" "$*" >&2; exit 1; }

# ── Defaults ────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG_FILE="${PROJECT_DIR}/configs/config.json"
EXAMPLE_CONFIG="${PROJECT_DIR}/configs/config.example.json"

NON_INTERACTIVE=false
AUTO_RUN=false

# Provider defaults (used in non-interactive or as prompts defaults)
LOCAL_ENABLED=true
LOCAL_BASE_URL="http://127.0.0.1:11434/v1"
LOCAL_MODEL="openclaw-local"
LOCAL_API="chat-completions"
LOCAL_TIMEOUT="12s"

CLOUD_ENABLED=false
CLOUD_BASE_URL="https://api.openai.com/v1"
CLOUD_MODEL="gpt-4.1"
CLOUD_API="chat-completions"
CLOUD_API_KEY=""
CLOUD_API_KEY_ENV="OPENAI_API_KEY"
CLOUD_TIMEOUT="45s"

SERVER_HOST="0.0.0.0"
SERVER_PORT=8080
LOG_LEVEL="info"

# ── Parse Arguments ─────────────────────────────────────────────────
usage() {
  cat <<EOF
${BOLD}Usage:${RESET}
  $(basename "$0") [options]

${BOLD}Options:${RESET}
  -y, --non-interactive    Run without prompts (use env vars or defaults)
  -r, --run                Start the gateway after configuration
  -o, --output FILE        Output config file path
                           (default: configs/config.json)
  -h, --help               Show this help message

${BOLD}Non-interactive environment variables:${RESET}
  LOCAL_ENABLED            Enable local provider (true/false)
  LOCAL_BASE_URL           Local provider base URL
  LOCAL_MODEL              Local model name
  LOCAL_API                Local API type (chat-completions/responses)
  CLOUD_ENABLED            Enable cloud provider (true/false)
  CLOUD_BASE_URL           Cloud provider base URL
  CLOUD_MODEL              Cloud model name
  CLOUD_API_KEY            Cloud API key (plain text)
  CLOUD_API_KEY_ENV        Env var name for cloud API key
  SERVER_PORT              Gateway listen port
  LOG_LEVEL                Log level (debug/info/warn/error)

${BOLD}Examples:${RESET}
  # Interactive setup
  $(basename "$0")

  # Non-interactive with env vars
  LOCAL_ENABLED=true CLOUD_ENABLED=true CLOUD_API_KEY=sk-xxx $(basename "$0") -y

  # Configure and run immediately
  $(basename "$0") -y -r
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -y|--non-interactive) NON_INTERACTIVE=true; shift ;;
    -r|--run)             AUTO_RUN=true; shift ;;
    -o|--output)          CONFIG_FILE="$2"; shift 2 ;;
    -h|--help)            usage ;;
    *)                    fail "Unknown option: $1\nRun '$(basename "$0") --help' for usage." ;;
  esac
done

# ── Env var overrides (for non-interactive mode) ────────────────────
LOCAL_ENABLED="${LOCAL_ENABLED:-${LOCAL_ENABLED}}"
LOCAL_BASE_URL="${LOCAL_BASE_URL:-${LOCAL_BASE_URL}}"
LOCAL_MODEL="${LOCAL_MODEL:-${LOCAL_MODEL}}"
LOCAL_API="${LOCAL_API:-${LOCAL_API}}"

CLOUD_ENABLED="${CLOUD_ENABLED:-${CLOUD_ENABLED}}"
CLOUD_BASE_URL="${CLOUD_BASE_URL:-${CLOUD_BASE_URL}}"
CLOUD_MODEL="${CLOUD_MODEL:-${CLOUD_MODEL}}"
CLOUD_API_KEY="${CLOUD_API_KEY:-${CLOUD_API_KEY}}"
CLOUD_API_KEY_ENV="${CLOUD_API_KEY_ENV:-${CLOUD_API_KEY_ENV}}"

SERVER_PORT="${SERVER_PORT:-${SERVER_PORT}}"
LOG_LEVEL="${LOG_LEVEL:-${LOG_LEVEL}}"

# ── Helpers ─────────────────────────────────────────────────────────
prompt_bool() {
  local label="$1" default="$2"
  local default_hint
  if [[ "$default" == "true" ]]; then
    default_hint="Y/n"
  else
    default_hint="y/N"
  fi
  while true; do
    printf "${CYAN}  ➜${RESET} ${label} [${default_hint}]: "
    read -r answer
    answer="${answer,,}"
    if [[ -z "$answer" ]]; then
      echo "$default"
      return
    fi
    case "$answer" in
      y|yes) echo "true"; return ;;
      n|no)  echo "false"; return ;;
      *)     warn "Please enter y or n." ;;
    esac
  done
}

prompt_string() {
  local label="$1" default="$2"
  printf "${CYAN}  ➜${RESET} ${label} [${default}]: "
  read -r answer
  if [[ -z "$answer" ]]; then
    echo "$default"
  else
    echo "$answer"
  fi
}

prompt_choice() {
  local label="$1"; shift
  local default="$1"; shift
  local options=("$@")

  printf "${CYAN}  ➜${RESET} ${label}\n"
  local i=1
  for opt in "${options[@]}"; do
    if [[ "$opt" == "$default" ]]; then
      printf "    ${GREEN}%d) %s (default)${RESET}\n" "$i" "$opt"
    else
      printf "    %d) %s\n" "$i" "$opt"
    fi
    ((i++))
  done

  while true; do
    printf "    Enter choice [1-%d]: " "${#options[@]}"
    read -r answer
    if [[ -z "$answer" ]]; then
      echo "$default"
      return
    fi
    if [[ "$answer" =~ ^[0-9]+$ ]] && [[ "$answer" -ge 1 ]] && [[ "$answer" -le "${#options[@]}" ]]; then
      echo "${options[$((answer-1))]}"
      return
    fi
    warn "Invalid choice."
  done
}

bool_to_json() {
  if [[ "$1" == "true" ]]; then echo "true"; else echo "false"; fi
}

# ── Pre-flight Checks ───────────────────────────────────────────────
printf "\n${BOLD}━━━ OpenClaw Gateway Setup ━━━${RESET}\n\n"

info "Running pre-flight checks..."

if ! command -v go &>/dev/null; then
  fail "Go is not installed. Please install Go 1.25+ first."
fi
GO_VERSION="$(go version | awk '{print $3}')"
ok "Go ${GO_VERSION##go} found"

if [[ -f "$CONFIG_FILE" ]]; then
  if [[ "$NON_INTERACTIVE" == "true" ]]; then
    warn "Existing config found at ${CONFIG_FILE}, overwriting."
  else
    OVERWRITE="$(prompt_bool "Config file already exists at ${CONFIG_FILE}. Overwrite?" "false")"
    if [[ "$OVERWRITE" != "true" ]]; then
      info "Keeping existing config. Exiting."
      exit 0
    fi
  fi
fi

if command -v lsof &>/dev/null; then
  if lsof -i ":${SERVER_PORT}" -sTCP:LISTEN &>/dev/null 2>&1; then
    warn "Port ${SERVER_PORT} is already in use."
  else
    ok "Port ${SERVER_PORT} is available"
  fi
fi

# ── Interactive Configuration ───────────────────────────────────────
if [[ "$NON_INTERACTIVE" == "false" ]]; then
  printf "\n${BOLD}── Provider Configuration ──${RESET}\n\n"

  LOCAL_ENABLED="$(prompt_bool "Enable local provider?" "$LOCAL_ENABLED")"

  if [[ "$LOCAL_ENABLED" == "true" ]]; then
    LOCAL_BASE_URL="$(prompt_string "Local provider base URL" "$LOCAL_BASE_URL")"
    LOCAL_MODEL="$(prompt_string "Local model name" "$LOCAL_MODEL")"
    LOCAL_API="$(prompt_choice "Local API type" "$LOCAL_API" "chat-completions" "responses")"
  fi

  printf ""
  CLOUD_ENABLED="$(prompt_bool "Enable cloud provider?" "$CLOUD_ENABLED")"

  if [[ "$CLOUD_ENABLED" == "true" ]]; then
    CLOUD_BASE_URL="$(prompt_string "Cloud provider base URL" "$CLOUD_BASE_URL")"
    CLOUD_MODEL="$(prompt_string "Cloud model name" "$CLOUD_MODEL")"
    CLOUD_API="$(prompt_choice "Cloud API type" "$CLOUD_API" "chat-completions" "responses")"

    printf "${CYAN}  ➜${RESET} Cloud API key (leave blank to use env var): "
    read -r CLOUD_API_KEY_INPUT
    if [[ -n "$CLOUD_API_KEY_INPUT" ]]; then
      CLOUD_API_KEY="$CLOUD_API_KEY_INPUT"
    else
      CLOUD_API_KEY_ENV="$(prompt_string "Env var name for API key" "$CLOUD_API_KEY_ENV")"
    fi
  fi

  printf "\n${BOLD}── Server Settings ──${RESET}\n\n"

  SERVER_PORT="$(prompt_string "Gateway port" "$SERVER_PORT")"
  LOG_LEVEL="$(prompt_choice "Log level" "$LOG_LEVEL" "debug" "info" "warn" "error")"
fi

# ── Validate ────────────────────────────────────────────────────────
if [[ "$LOCAL_ENABLED" != "true" && "$CLOUD_ENABLED" != "true" ]]; then
  fail "At least one provider must be enabled."
fi

if [[ "$CLOUD_ENABLED" == "true" && -z "$CLOUD_API_KEY" && -z "$CLOUD_API_KEY_ENV" ]]; then
  if [[ "$NON_INTERACTIVE" == "true" ]]; then
    warn "Cloud provider enabled but no API key configured."
  else
    fail "Cloud provider requires an API key or api_key_env."
  fi
fi

if ! [[ "$SERVER_PORT" =~ ^[0-9]+$ ]] || [[ "$SERVER_PORT" -lt 1 ]] || [[ "$SERVER_PORT" -gt 65535 ]]; then
  fail "Invalid port: ${SERVER_PORT}"
fi

# ── Generate Config ─────────────────────────────────────────────────
printf "\n${BOLD}── Generating Config ──${RESET}\n\n"

mkdir -p "$(dirname "$CONFIG_FILE")"

# Build JSON with printf to avoid jq dependency
LOCAL_BLOCK=""
if [[ "$LOCAL_ENABLED" == "true" ]]; then
  LOCAL_BLOCK=$(cat <<EOF
    "local": {
      "enabled": true,
      "type": "openclaw",
      "api": "${LOCAL_API}",
      "base_url": "${LOCAL_BASE_URL}",
      "model": "${LOCAL_MODEL}",
      "timeout": "${LOCAL_TIMEOUT}",
      "retry_attempts": 0,
      "retry_backoff": "0s"
    }
EOF
)
else
  LOCAL_BLOCK=$(cat <<EOF
    "local": {
      "enabled": false
    }
EOF
)
fi

CLOUD_BLOCK=""
if [[ "$CLOUD_ENABLED" == "true" ]]; then
  API_KEY_LINE=""
  if [[ -n "$CLOUD_API_KEY" ]]; then
    API_KEY_LINE="\"api_key\": \"${CLOUD_API_KEY}\","
  fi
  API_KEY_ENV_LINE=""
  if [[ -n "$CLOUD_API_KEY_ENV" ]]; then
    API_KEY_ENV_LINE="\"api_key_env\": \"${CLOUD_API_KEY_ENV}\","
  fi
  CLOUD_BLOCK=$(cat <<EOF
    "cloud": {
      "enabled": true,
      "type": "openai-compatible",
      "api": "${CLOUD_API}",
      "base_url": "${CLOUD_BASE_URL}",
      "model": "${CLOUD_MODEL}",
      ${API_KEY_LINE}
      ${API_KEY_ENV_LINE}
      "timeout": "${CLOUD_TIMEOUT}",
      "retry_attempts": 2,
      "retry_backoff": "300ms"
    }
EOF
)
else
  CLOUD_BLOCK=$(cat <<EOF
    "cloud": {
      "enabled": false
    }
EOF
)
fi

cat > "$CONFIG_FILE" <<EOF
{
  "version": "v1",
  "server": {
    "host": "${SERVER_HOST}",
    "port": ${SERVER_PORT},
    "read_timeout": "10s",
    "write_timeout": "60s"
  },
  "routing": {
    "default_mode": "local",
    "sticky_ttl": "30m",
    "cloud_dwell_time": "15m",
    "complexity_threshold": 0.75,
    "confidence_threshold": 0.55,
    "local_context_limit": 8192,
    "offline_force_local": true
  },
  "providers": {
${LOCAL_BLOCK},
${CLOUD_BLOCK}
  },
  "observability": {
    "log_level": "${LOG_LEVEL}",
    "decision_log": true
  }
}
EOF

ok "Config written to ${CONFIG_FILE}"

# ── Build & Optional Run ───────────────────────────────────────────
printf "\n${BOLD}── Build ──${RESET}\n\n"

info "Building binary..."
cd "$PROJECT_DIR"
if go build -o "${PROJECT_DIR}/bin/openclaw-gateway" ./cmd/gateway 2>&1; then
  ok "Build succeeded"
else
  fail "Build failed."
fi

printf "\n${BOLD}── Done ──${RESET}\n\n"

if [[ "$AUTO_RUN" == "true" ]]; then
  info "Starting gateway..."
  exec "${PROJECT_DIR}/bin/openclaw-gateway" -config "$CONFIG_FILE"
else
  printf "  Start the gateway:\n"
  printf "    ${GREEN}make run${RESET}\n"
  printf "  Or directly:\n"
  printf "    ${GREEN}./bin/openclaw-gateway -config ${CONFIG_FILE}${RESET}\n\n"
fi
