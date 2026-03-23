# Contributing

## Before you open a PR

Start with an issue when the change affects routing behavior, protocol
compatibility, installation flow, or public configuration shape. Small fixes
can go directly to a pull request.

## Development workflow

1. Fork the repository and create a focused branch.
2. Make the smallest coherent change that solves one problem.
3. Run local checks before pushing:

```bash
gofmt -w ./cmd ./internal
go test ./...
go build -o bin/openclaw-gateway ./cmd/gateway
```

4. Update documentation when behavior or config changes.

## Contribution rules

- Keep changes scoped. Large refactors should be split into reviewable pieces.
- Preserve protocol compatibility unless the change is explicitly discussed.
- Add or update tests for routing, translation, and config validation changes.
- Do not commit secrets, private endpoints, or local machine paths.
- Prefer configuration examples over hard-coded environment assumptions.

## Pull request checklist

- The change is explained clearly in the PR description.
- Tests were added or updated when behavior changed.
- Documentation was updated when user-facing behavior changed.
- Config changes remain backward compatible, or the breaking change is called
  out explicitly.

## Issue quality bar

Bug reports should include:

- Expected behavior.
- Actual behavior.
- Config snippet with secrets removed.
- Reproduction steps.
- Local model/runtime details when relevant.

Feature requests should include:

- The user problem.
- Why the current gateway flow is insufficient.
- Why the change belongs in the core project instead of a private fork.

## Release policy

Maintainers may squash-merge PRs to keep history readable. By submitting a
contribution, you agree that your contribution will be licensed under the
project license.

