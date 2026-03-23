# OpenClaw Gateway

[中文文档](README.zh-CN.md)

OpenClaw Gateway is a local-first routing gateway for hybrid inference.

The project sits between client applications and model providers, decides whether a request should stay on the device or be escalated to the cloud, and exposes a single OpenAI-compatible entrypoint to the caller.

## Why this project exists

Hybrid inference is useful only when routing is predictable and operable.

This project focuses on four practical outcomes:

- Keep sensitive or offline requests on the local stack.
- Escalate harder requests to a cloud provider when local quality is not enough.
- Prevent route flapping with sticky sessions and dwell time.
- Give operators a clean place to observe why a route decision was made.

## MVP scope

Version `v0.1.0` targets a narrow but useful first release:

- One binary HTTP gateway written in Go.
- OpenAI-compatible `POST /v1/chat/completions` entrypoint.
- OpenAI-compatible `POST /v1/responses` entrypoint for OpenClaw-style provider wiring.
- Local provider support for OpenClaw through an OpenAI-compatible endpoint.
- Cloud provider support for an OpenAI-compatible upstream.
- Rule-based routing with privacy, offline, complexity, confidence, and context length signals.
- Sticky session routing to avoid rapid back-and-forth switching.
- Decision inspection endpoint for debugging and future dashboard work.

## Non-goals for the first release

- Multi-agent orchestration.
- Streaming responses.
- Visual workflow builder.
- Full dashboard UI.
- Distributed session storage.

## Architecture

The first release has a simple shape:

- `client -> gateway -> local provider or cloud provider`
- The gateway is both the policy enforcement point and the protocol boundary.
- The dashboard, when added later, should remain control-plane only and must not sit on the critical request path.

Detailed notes live in [docs/architecture.md](docs/architecture.md).

## Installation

You can run the project in three ways:

- From source with `go run`
- As a local binary with `go build`
- With Docker Compose through [deploy/docker-compose.yaml](deploy/docker-compose.yaml)

Detailed steps live in [docs/install.md](docs/install.md).

## Repository layout

```text
cmd/gateway            Entrypoint and process lifecycle
internal/config        Config loading, validation, defaults
internal/policy        Pure routing rules
internal/router        Sticky-session decision orchestration
internal/providers     Upstream provider abstraction and adapters
internal/session       In-memory sticky session store
internal/server        HTTP handlers and request parsing
internal/telemetry     Structured logging
configs                Example configuration
docs                   Product spec and architecture notes
```

## Quick start

1. Copy `configs/config.example.json` to a local config file.
2. Point the local provider at your OpenClaw OpenAI-compatible endpoint.
3. Export the cloud provider API key if you want cloud fallback.
4. Start the gateway:

```bash
make run
```

5. Check health:

```bash
curl -s http://127.0.0.1:8080/healthz
```

6. Inspect a route decision:

```bash
curl -s http://127.0.0.1:8080/v1/route/decision \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "system", "content": "You are a routing test."},
      {"role": "user", "content": "Summarize a private local document."}
    ],
    "metadata": {
      "session_id": "demo-1",
      "privacy_level": "high",
      "complexity": 0.2
    }
  }'
```

7. Send a chat completion request through the gateway:

```bash
curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "Explain how to build a hybrid routing gateway."}
    ],
    "metadata": {
      "session_id": "demo-2",
      "complexity": 0.8
    }
  }'
```

8. Send a responses request through the gateway:

```bash
curl -s http://127.0.0.1:8080/v1/responses \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "auto",
    "input": [
      {"role": "user", "content": "Explain how OpenClaw should connect to this gateway."}
    ],
    "metadata": {
      "session_id": "demo-3",
      "complexity": 0.4
    }
  }'
```

## Decision signals

The current rule engine consumes these signals:

- `privacy_level`: `high` or `sensitive` forces local mode.
- `offline`: when true and `offline_force_local` is enabled, force local mode.
- `complexity`: if it crosses the configured threshold, route to cloud.
- `local_confidence`: if local confidence is below the configured threshold, route to cloud.
- `estimated_tokens`: derived from request messages and compared with the local context limit.
- `session_id`: enables sticky routing and cloud dwell time.

## Configuration

The gateway uses JSON config in the first release to avoid extra runtime dependencies.

An example file is available at [configs/config.example.json](configs/config.example.json).

Cloud secrets can be supplied either inline for development or through `api_key_env` for normal use.

For upstream resilience, each provider can also define:

- `retry_attempts`: how many extra attempts should be made for retryable upstream failures.
- `retry_backoff`: delay between retry attempts.

The current implementation retries `502`, `503`, and `504`, and returns a
structured JSON error instead of passing HTML error pages through to clients.

## Open Source Workflow

Repository governance and contribution flow are now defined in:

- [LICENSE](LICENSE)
- [CONTRIBUTING.md](CONTRIBUTING.md)
- [SECURITY.md](SECURITY.md)
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

GitHub automation and community templates live under `.github/`:

- CI workflow for formatting, test, and build checks
- Tag-driven release workflow for cross-platform binaries
- Bug report and feature request issue forms
- Pull request template

## Packaging

The repository includes:

- [Dockerfile](Dockerfile) for container builds
- [docker-compose.yaml](deploy/docker-compose.yaml) for local compose-based startup
- A release workflow that packages Linux, macOS, and Windows binaries on tags like `v0.1.0`

## Roadmap

The current implementation covers stage 0 and the first slice of stage 1:

- Stage 0: product scope, architecture, config schema.
- Stage 1: runnable gateway, health endpoint, rule engine, local and cloud provider adapters.
- Stage 2: provider retries, metrics, persistent session storage.
- Stage 3: dashboard, policy editor, release packaging.

OpenClaw wiring notes live in [docs/openclaw.md](docs/openclaw.md).

Further planning notes live in [docs/roadmap.md](docs/roadmap.md).

Interactive presentation: [docs/presentation.html](docs/presentation.html)
