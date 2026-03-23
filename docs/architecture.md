# Architecture Notes

## System shape

The project is intentionally split into a data plane and a future control plane.

Data plane in the first release:

- Accept OpenAI-compatible chat requests from clients.
- Evaluate local-vs-cloud routing policy.
- Forward the request to the selected provider.
- Return the upstream response to the caller.

Control plane in later releases:

- Manage policy configuration.
- Manage provider definitions.
- Show decision logs, latency, failure rate, and cost metrics.
- Push config changes to one or more gateway instances.

## Core design decisions

### 1. One gateway can serve many sessions

The gateway is a routing node, not an agent runtime. It can process many sessions at once as long as session state is externalized or scoped cleanly.

### 2. Session stickiness is mandatory

Without stickiness, hybrid routing turns into route flapping. This first version uses an in-memory sticky session table. A later version should move this to Redis when multiple gateway nodes are introduced.

### 3. Rule-based policy first

The first release should use transparent rules before any learned decision model is introduced. Operators need to know why a request was routed.

### 4. OpenAI-compatible wire format first

Supporting one protocol end-to-end keeps the first release narrow. Anthropic-style and other provider shapes can be added later through protocol adapters.

## Request path

1. Client sends `POST /v1/chat/completions`.
2. Server extracts routing signals from request metadata and headers.
3. Router checks sticky session state.
4. Policy engine evaluates the request when there is no sticky route.
5. Gateway selects the local or cloud provider adapter.
6. Upstream response is returned to the client with route headers.

## Routing rules in v0.1

The default precedence order is:

1. Offline forced local.
2. High privacy forced local.
3. Context too large for local goes cloud.
4. Complexity too high goes cloud.
5. Local confidence too low goes cloud.
6. Fall back to default mode.

## API notes

Supported now:

- `GET /healthz`
- `POST /v1/route/decision`
- `POST /v1/chat/completions`

Not supported yet:

- Streaming.
- Embeddings.
- Tools.
- Multi-provider retries.
- Metrics endpoint.

