# ADR-0005: Drive Compose via the CLI; read state via the Engine API

- Status: Accepted (2026-11-20)

## Context

Each app is one Compose project (`hub-<id>`), so an app can grow extra services (e.g. a database) without platform changes. The core needs up/down plus live status, events and logs.

## Options

| Option | Finding |
|---|---|
| Embed the official Compose SDK (`docker/compose/v5`) | Supported as a library, but Docker's own docs require pinning `github.com/docker/cli v28.5.2+incompatible` because CLI 29+ depends on `moby/moby` while Compose v5 depends on `docker/docker` → dependency conflicts on Docker releases. |
| Docker Engine API only (reimplement compose) | Reinvents Compose semantics. |
| **`docker compose` CLI for up/down/build + Engine Go client for ps/events/logs/inspect** | Always matches the installed Docker; typed status from the Engine API. Runtipi also uses one Compose project per app. |

## Decision

Runtime driver interface in core: `Start`, `Stop`, `Status`, `Logs`, `Events`. The `docker` driver shells out to `docker compose` with a core-generated override file and uses the Engine Go client for everything read-only. A `process` driver (native dev mode) comes in Phase 2.

## Consequences

- No Compose SDK dependency churn in the core's `go.mod`.
- Must parse CLI exit codes/stderr for up/down errors.
- The SDK can replace the CLI behind the interface later without other changes.

## Sources

- https://docs.docker.com/compose/compose-sdk.md
- https://runtipi.io/docs/reference/architecture
