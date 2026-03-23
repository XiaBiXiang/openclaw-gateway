# Roadmap

## Milestone 0

Freeze product shape.

- Define project scope.
- Define config schema.
- Define API boundary.
- Define routing signals and non-goals.

## Milestone 1

Build a usable local binary.

- Start and stop cleanly.
- Load config.
- Expose health and decision endpoints.
- Proxy OpenAI-compatible chat requests to local or cloud.

## Milestone 2

Make it robust enough for real use.

- Retries and circuit breaking.
- Request and provider timeouts.
- Persistent session store.
- Metrics and structured decision logs.

## Milestone 3

Make it publishable.

- GitHub Actions.
- Docker image and compose file.
- Cross-platform build scripts.
- Install documentation.

## Milestone 4

Add control-plane features.

- Dashboard.
- Policy editor.
- Multi-instance management.
- History and policy rollout.

