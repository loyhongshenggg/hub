# ADR-0007: Nx 24 + pnpm + go.work + @naxodev/gonx

- Status: Accepted (2026-11-20)

## Context

One repo holds Go (core, app backends, go-appkit) and TypeScript (shell, app UIs, SDK, ui-kit, generator). `nx affected` must see changes across both languages.

## Options for Go in Nx

| Option | Finding |
|---|---|
| `@nx-go/nx-go` | Compatibility table ends at Nx 18 — stale. |
| Hand-written `nx:run-commands` | Works, but Nx can't see Go module dependencies → `affected` misses cross-module changes. |
| **`@naxodev/gonx` 5.x** | Maintained fork, in Nx's community plugin registry, requires Nx 24. Inferred build/test/lint/tidy/serve targets, cacheable, Go dependency detection. |

## Decision

- Nx 24 (current major, released 2026-10-21; 18-month support window).
- pnpm workspaces for TS packages; `go.work` for Go modules.
- `@naxodev/gonx` 5.x for Go projects.
- Node 26 LTS, Go latest stable.
- Nx Cloud off (local only).
- Module boundaries via tags: apps → `libs/*` only.

## Consequences

- One `nx affected -t test` covers both languages.
- gonx relies on inferred tasks, so dropping it later means writing `run-commands` targets — low lock-in.

## Sources

- https://nx.dev/docs/reference/releases
- https://github.com/nrwl/nx/pull/31034
- https://cdn.jsdelivr.net/npm/@naxodev/gonx/README.md
- https://npmjs.com/package/@nx-go/nx-go
- https://nodejs.org/en/about/previous-releases
