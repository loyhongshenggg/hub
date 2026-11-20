# ADR-0008: Dev mode runs app processes on the host

- Status: Accepted (2026-11-21)

## Context

Phase 2 needs a fast inner loop: edit an app and see the change inside the shell without rebuilding its image (20–60 s per change in Run mode). The first app built this way is `radar`.

## Options

| Option                                                                                  | UI edit visible                                            | Go edit live                                                 | Per-app setup                                            | Fidelity                                                      |
| --------------------------------------------------------------------------------------- | ---------------------------------------------------------- | ------------------------------------------------------------ | -------------------------------------------------------- | ------------------------------------------------------------- |
| **Host processes run by the core** (Vite dev server + `go run`, gateway routes to them) | Instant (Vite HMR through the gateway)                     | ~0.6 s measured (watch → `go run` restart)                   | A `dev:` block in the manifest (the generator writes it) | Not containerised; Run mode and `hub validate` stay on Docker |
| Docker Compose Watch (`develop.watch`: sync / sync+restart / rebuild)                   | Instant only with a separate dev image running Vite inside | Image rebuild, or a Go toolchain + sync+restart in the image | Dev Dockerfile target + watch rules per app              | Close to production                                           |
| Rebuild the image on each change                                                        | 20–60 s                                                    | 20–60 s                                                      | None                                                     | Exact                                                         |

Compose Watch is a good fit for single-image apps without a frontend build, but every app here would need a second, dev-only image. Host processes reuse the toolchain already on the machine.

For restarting Go on change, [air](https://github.com/air-verse/air) is the usual tool. We use a small polling watcher in the core instead (`dev.processes[].watch` patterns): no extra binary to download and run, and it works for any language.

## Decision

- Manifest v1 gains an optional `dev:` block (additive, no version bump): `route` + `processes[]` with `name`, `dir`, `command`, `watch`.
- The core's `process` driver runs each process as `/bin/sh -c` in its own process group. Each gets a free port in `$PORT` and sees its siblings' ports as `$HUB_PORT_<NAME>`, plus `HUB_APP_ID`, `HUB_APP_URL`, `HUB_CORE_URL`, `HUB_DEV=1`, `HUB_DATA_DIR` (`~/.local/share/hub/dev/<id>/data`).
- The gateway routes `<id>.hub.localhost` to the `route` process. Health uses the same `health.path` (Vite proxies it to the API), with at least 2 minutes for the first compile.
- A process with `watch` patterns restarts when a matching file changes. If it exits (e.g. a compile error) it waits for the next change instead of failing the app. A process without `watch` that exits puts the app in `error`.
- Dev instances don't outlive the core: it stops them on shutdown. Pid files with process start times let the next core kill groups left by a crash. Linux never reuses a PID while it is a live process group ID, so surviving members are safe to kill even when the leader is gone.
- The shell shows a DEV tag. "Start in dev mode" / "Switch to …" live in the app menu and on the off panel. `hub apps start|restart <id> --dev` works from the CLI.

## Consequences

- Inner loop: UI edits are instant, Go edits ~1 s, no Docker build.
- Dev mode needs the app's toolchain on the host (Go, pnpm). The WSL setup script already installs it.
- Dev data is separate from the Run-mode volume, so experiments don't touch real data.
- `hub validate` and Run mode remain the source of truth for "does it work in a container".

## Sources

- https://docs.docker.com/compose/how-tos/file-watch/
- https://docs.docker.com/reference/compose-file/develop/
- https://oneuptime.com/blog/post/2026-01-16-docker-compose-watch/view
- https://github.com/air-verse/air
- https://blog.brightcoding.dev/2026/06/02/stop-killing-your-go-productivity-why-developers-are-ditching-manual-restarts-for-air
