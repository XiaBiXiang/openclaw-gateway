# Installation

## Option 1: Run from source

Requirements:

- Go 1.25 or newer.

Steps:

1. Copy `configs/config.example.json` to your own config file.
2. Update local and cloud provider settings.
3. Export any required provider API keys.
4. Start the gateway:

```bash
go run ./cmd/gateway -config /path/to/config.json
```

If the cloud upstream is not fully stable, set `retry_attempts` and
`retry_backoff` on the provider config to absorb transient `502/503/504`
failures.

## Option 2: Build a local binary

```bash
go build -o bin/openclaw-gateway ./cmd/gateway
./bin/openclaw-gateway -config /path/to/config.json
```

For a local-first MLX plus OpenClaw setup, start from the tracked example:

```bash
cp configs/config.local.example.json configs/config.local.json
```

Update the local model id, the cloud upstream, and the cloud `api_key_env`
inside `configs/config.local.json` if needed.

The helper scripts assume:

- `conda` is installed
- the `mlx-lm` package is available in the `mlx-lm` conda environment
- your local model server should listen on `127.0.0.1:8000`

If you want one command for both the MLX server and the gateway, use:

```bash
MLX_MODEL=mlx-community/Qwen3-4B-Instruct-2507-4bit ./scripts/run-hybrid-stack.sh
```

`MLX_MODEL` can be either a Hugging Face repo id or a local model directory.

That command starts the local MLX model server if needed and then starts the
gateway with `configs/config.local.json` or, if that file does not exist,
`configs/config.local.example.json`.

If you want to run the two processes separately, use:

```bash
./scripts/run-mlx-server.sh mlx-community/Qwen3-4B-Instruct-2507-4bit
./scripts/run-local.sh
```

`run-mlx-server.sh` accepts either a Hugging Face repo id or a local model
path.

`run-local.sh` reads the cloud `api_key_env` from the selected gateway config.
If that env var is already exported, it uses it directly. If the env var is
missing and `OPENCLAW_PROVIDER_ID` is set, the script tries to pull the API key
from `~/.openclaw/openclaw.json`.

Example:

```bash
export OPENAI_API_KEY=replace-me
./scripts/run-local.sh
```

Or reuse a key already stored in OpenClaw:

```bash
export OPENCLAW_PROVIDER_ID=openai
./scripts/run-local.sh
```

## Option 3: Run with Docker Compose

1. Copy `deploy/.env.example` to `deploy/.env`.
2. Update `configs/config.example.json` or mount your own config file.
3. Start the stack:

```bash
cd deploy
docker compose up --build
```

The compose file mounts `configs/config.example.json` as `/app/config.json`.
Replace that example file with your own configuration before using it for
real traffic.

## Verify the process

```bash
curl -s http://127.0.0.1:8080/healthz
```

## OpenClaw wiring

See [docs/openclaw.md](openclaw.md) for provider wiring examples.
