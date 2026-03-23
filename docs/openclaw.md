# OpenClaw Integration

## Goal

Use this gateway as a custom model provider inside OpenClaw so OpenClaw can talk to a single local endpoint while the gateway decides whether to use a local model or a cloud model.

## Required gateway API

OpenClaw on this machine is already configured around `openai-responses`, so the gateway exposes:

- `POST /v1/responses`

The gateway can then translate requests to a local `chat/completions` backend when needed.

## Example gateway config

```json
{
  "providers": {
    "local": {
      "enabled": true,
      "type": "openclaw",
      "api": "chat-completions",
      "base_url": "http://127.0.0.1:11434/v1",
      "model": "openclaw-local",
      "timeout": "12s"
    },
    "cloud": {
      "enabled": true,
      "type": "openai-compatible",
      "api": "responses",
      "base_url": "https://api.openai.com/v1",
      "model": "gpt-4.1",
      "api_key_env": "OPENAI_API_KEY",
      "timeout": "45s"
    }
  }
}
```

## Example OpenClaw provider entry

The exact file format is controlled by OpenClaw, but the provider shape should conceptually look like this:

```json
{
  "my-local-router": {
    "baseUrl": "http://127.0.0.1:18080/v1",
    "apiKey": "not-required-unless-you-enable-auth",
    "api": "openai-responses",
    "models": [
      {
        "id": "auto",
        "name": "Auto Route",
        "reasoning": true,
        "input": ["text"],
        "contextWindow": 128000,
        "maxTokens": 32768
      }
    ]
  }
}
```

## Recommended wiring

1. Keep OpenClaw pointed only at this gateway.
2. Let the gateway choose `local` or `cloud`.
3. Keep secrets in gateway env vars, not in OpenClaw provider config, whenever possible.
4. Start with `default_mode=local` and cloud fallback rules.

## Local bootstrap helper scripts

The repository also includes:

- `configs/config.local.example.json`
- `scripts/run-local.sh`
- `scripts/run-mlx-server.sh`
- `scripts/run-hybrid-stack.sh`

Recommended flow:

1. Copy `configs/config.local.example.json` to `configs/config.local.json`.
2. Update the cloud provider and model settings for your environment.
3. Export the env var referenced by `providers.cloud.api_key_env`.
4. Start the gateway.

```bash
cp configs/config.local.example.json configs/config.local.json
export OPENAI_API_KEY=replace-me
./scripts/run-local.sh
```

If you already store the cloud key in OpenClaw, you can point the script at a
provider entry instead of exporting the key yourself:

```bash
export OPENCLAW_PROVIDER_ID=openai
./scripts/run-local.sh
```

In that mode, `run-local.sh` reads the cloud `api_key_env` from the selected
gateway config, resolves the matching `apiKey` from
`~/.openclaw/openclaw.json`, exports it, and then starts the gateway.

For an MLX-backed local model server plus gateway in one command:

```bash
MLX_MODEL=mlx-community/Qwen3-4B-Instruct-2507-4bit ./scripts/run-hybrid-stack.sh
```

`run-mlx-server.sh` accepts either a local model directory or a Hugging Face
repo id. `run-hybrid-stack.sh` starts the MLX OpenAI-compatible endpoint on
`127.0.0.1:8000/v1` if needed, then starts the gateway on `127.0.0.1:18080`
using the local-first example config.
