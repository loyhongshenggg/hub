# hub — architecture

A locally hosted, Lark-style personal platform. One shell UI hosts many small apps. Each app has a power button (starts/stops its containers) and opens in a tab.

Status: design approved 2026-11-20. Decisions with research and sources live in [`adr/`](adr/).

## 1. The three parts

| Part | What it is | Where |
|---|---|---|
| **Shell** | React UI: dock of apps, power toggles, tabs. Each tab is an iframe. | `platform/shell` |
| **Core** | One Go binary: finds apps, starts/stops their containers, health-checks them, reverse-proxies their traffic, serves the shell, exposes the `hub` CLI. | `platform/core` |
| **Apps** | One folder each with `app.manifest.yaml` + `compose.yaml`. Runs in its own container(s). | `apps/<id>` |

**The whole plug-in contract:** an app is a web server in a container, has a health endpoint, and ships an `app.manifest.yaml`. Everything else (SDK, ui-kit, appkit) is optional.

## 2. Runtime picture

```
Windows 11
 └─ Chrome/Edge → http://hub.localhost:7000      (WSL forwards localhost)

WSL2 · Ubuntu        (repo at ~/code/hub on the Linux filesystem)
 ├─ hub core (Go) :7000 ── shell UI · /api · gateway
 │      │  docker.sock  (docker compose CLI + Engine API)
 │      ▼
 └─ Docker Engine ── private network "hub"
        ├─ hub-notes:  web 172.20.0.5:8080 · postgres
        └─ hub-budget: web 172.20.0.7:8080
```

```
Browser ── http://hub.localhost:7000
┌──────────────────────────────────────────────────────────────┐
│ SHELL (React)                                                │
│ ┌──────────┐ ┌─────────────────────────────────────────────┐ │
│ │ Dock     │ │ [Home] [Notes ●] [Budget ●]          tabs   │ │
│ │ ● Notes  │ ├─────────────────────────────────────────────┤ │
│ │ ● Budget │ │  iframe → http://notes.hub.localhost:7000   │ │
│ │ ○ Todo   │ │  (other running tabs stay mounted, hidden)  │ │
│ └──────────┘ └─────────────────────────────────────────────┘ │
└───────────┬─────────────────────────────────┬────────────────┘
  REST + SSE│ /api/...                        │ app traffic, routed by Host header
            ▼                                 ▼
┌──────────────────────────────────────────────────────────────┐
│ CORE (Go)                                                    │
│  Registry   reads apps/*/app.manifest.yaml                   │
│  Lifecycle  on/off state machine, health checks, reconcile   │
│  Runtime    driver interface → docker (now), process (later) │
│  Gateway    reverse proxy  notes.hub.localhost → container   │
│  Store      SQLite: tab order, pins, autostart               │
│  Serves the built shell (go:embed)                           │
└──────────────────────────────────────────────────────────────┘
```

## 3. Addressing

- Shell: `http://hub.localhost:7000`. Plain `http://localhost:7000` redirects there.
- App: `http://<id>.hub.localhost:7000`. Apps are served at `/`.
- Why `hub.localhost` and not bare `localhost`: Chrome computes `notes.hub.localhost` and `hub.localhost` as the same site (`hub.localhost`), so cookies/logins work inside the iframe, while different origins keep apps isolated. Bare `localhost` vs `notes.localhost` would be cross-site. See [ADR-0002](adr/0002-subdomain-routing.md).
- Core listens on `127.0.0.1:7000` only. Nothing is exposed to the network.

## 4. App contract

```yaml
# apps/notes/app.manifest.yaml
manifestVersion: 1
id: notes                 # → notes.hub.localhost ; [a-z][a-z0-9-]*
name: Notes
description: Markdown notes
icon: ./icon.svg
runtime:
  driver: docker
  compose: ./compose.yaml
  service: web            # compose service the gateway routes to
  port: 8080              # port inside the container
health:
  path: /healthz
  startTimeout: 60s
ui:
  path: /                 # page opened in the tab
  keepAlive: true         # keep iframe mounted when switching tabs
autoStart: false
```

```
apps/notes/
├─ app.manifest.yaml
├─ compose.yaml       # only the app's own services; core adds platform wiring
├─ Dockerfile         # multi-stage: build web → build Go → small final image
├─ project.json       # Nx targets: dev, build, test, lint, docker-build, validate
├─ api/               # Go module, uses libs/go-appkit
└─ web/               # React + Vite, uses @hub/app-sdk (optional)
```

Rules:

1. One HTTP port, inside the container. No host ports published.
2. `GET <health.path>` returns 2xx when ready.
3. Serve the UI at `/`. Absolute paths are fine (each app owns its origin).
4. Persist data in named volumes. Power off keeps volumes.
5. The manifest JSON Schema in `libs/manifest` is the source of truth; `manifestVersion` bumps on breaking changes.

### Core-injected wiring

At start, the core passes an extra compose override file so app compose files stay clean:

- joins the target service to the `hub` network
- labels: `hub.app.id=<id>`, `hub.managed=true`
- env: `HUB_APP_ID`, `HUB_APP_URL`, `HUB_CORE_URL`

Compose project name: `hub-<id>`.

## 5. Lifecycle

States: `stopped → starting → running → stopping → stopped`, plus `error`.

| Action | What the core does |
|---|---|
| Power on | `docker compose -p hub-<id> -f compose.yaml -f <override> up -d` (builds image on first start) → resolve container IP on `hub` network → poll health until 2xx or `startTimeout` → `running` |
| Power off | `docker compose -p hub-<id> down` (volumes kept) → `stopped` |
| Crash | Docker event `die` → `error`, logs available |
| Core restart | list containers with `hub.app.id` label → re-adopt running apps |

Live status reaches the shell over SSE.

Each app has a worker that drives its actual state toward the requested one, so overlapping power presses resolve in order (off during a start cancels the health wait, then runs `down`). Failed starts and crashes stay in `error` (containers kept for logs) until the next power press. Shutting the core down leaves apps running; the next start re-adopts them.

## 6. Core API (shell-facing)

```
GET   /api/apps                        {apps: AppInfo[], problems: [{id, path, error}]}
GET   /api/apps/{id}                   AppInfo
POST  /api/apps/{id}/start|stop|restart   202 AppInfo (progress via /api/events)
PATCH /api/apps/{id}                   prefs: {pinned?, position?, autoStart?: bool|null}
GET   /api/apps/{id}/icon              icon file (sandboxed CSP)
GET   /api/apps/{id}/logs?tail=200&follow=1   SSE "log" events {service, stream, text}
GET   /api/events                      SSE "apps" event: full list on connect + after every change
GET   /api/state/{key}  PUT …          opaque shell UI state (JSON), e.g. open tabs
```

Request guards: unknown `Host` headers get 421 (DNS-rebinding defence); non-GET `/api` calls with a foreign `Origin` or a `Sec-Fetch-Site` other than `same-origin`/`none` get 403, so app iframes can't drive the API. Apps may only be framed by the shell (`frame-ancestors http://hub.localhost:7000`).

CLI: `hub serve`, `hub apps ls|start|stop|restart|logs <id>`, `hub validate <id>`. The CLI talks to the running core at `HUB_ADDR` (default `127.0.0.1:7000`). Data (SQLite, generated compose overrides) lives in `~/.local/share/hub`.

`hub validate <id>`: schema-check the manifest → build → start a throwaway `hub-validate-<id>` project → wait for health → `GET ui.path` must be 2xx `text/html` → `down --volumes`. Pass/fail, with the last log lines on failure.

`HUB_CORE_URL` is the browser-facing shell URL; the core listens on loopback only, so containers can't call it directly (Phase 3 platform services will add a path).

## 7. Shell ⇄ app bridge (`@hub/app-sdk`, optional)

postMessage, typed, versioned protocol.

- shell → app: context (`appId`, theme, locale), theme changes
- app → shell: `ready()`, `setBadge(n)`, `notify(msg)`, `openApp(id, path)`, `setTitle()`

The gateway strips `X-Frame-Options` / `frame-ancestors` from app responses so apps can be framed by the shell.

## 8. Monorepo layout

```
hub/
├─ platform/
│  ├─ core/            Go: control plane, gateway, `hub` CLI
│  └─ shell/           React shell UI
├─ apps/               every pluggable app, one folder each
│  └─ hello/           reference app
├─ libs/
│  ├─ app-sdk/         TS: shell⇄app bridge (@hub/app-sdk)
│  ├─ ui-kit/          shared React components + design tokens (@hub/ui-kit)
│  ├─ go-appkit/       Go: health, config, graceful shutdown, static serving
│  └─ manifest/        JSON Schema for app.manifest.yaml
├─ tools/hub-plugin/   Nx local plugin: `nx g @hub/plugin:app <id>`
├─ .claude/skills/hub-app/   skill: create or plug in an app
├─ docs/               this file, adr/, adding-an-app.md
└─ nx.json · package.json · pnpm-workspace.yaml · go.work
```

Module boundaries (Nx tags + `@nx/enforce-module-boundaries`): apps may depend on `libs/*` only — never on other apps or on `platform/*`.

## 9. Stack

| Layer | Choice |
|---|---|
| Dev environment | WSL2 Ubuntu 26.04, repo on Linux FS, VS Code WSL extension |
| Containers | Docker Engine + Compose plugin inside Ubuntu |
| Monorepo | Nx 24, pnpm workspaces, `go.work`, `@naxodev/gonx` 5.x for Go projects |
| Core | Go (latest stable), stdlib `net/http` + `httputil.ReverseProxy`, Docker Engine Go client, `modernc.org/sqlite`, SSE |
| Shell | React + TypeScript + Vite, TanStack Query, Zustand, Tailwind + shadcn/ui |
| App template | Go API embedding a React/Vite UI → one container, one port |
| Tests | Go `testing`, Vitest; Playwright for shell e2e later |

## 10. Phases

1. **Phase 0 — environment.** WSL2 + Ubuntu, Git, Go, Node 26, pnpm, Docker Engine. Repo created.
2. **Phase 1 — foundations.** Nx workspace; core (registry, docker driver, lifecycle, gateway, SSE, SQLite, CLI + validate); shell (dock, home grid, power toggles, keep-alive tabs, logs drawer, light/dark); libs; `hello` app; generator; skill; docs.
3. **Phase 2 — dev ergonomics.** `process` driver (hot reload inside the shell), rebuild button, CPU/RAM per app, start on Windows login.
4. **Phase 3 — platform services.** App↔app event bus, notification centre, global search, shared settings/secrets.

## 11. Out of scope (for now)

- Access from other devices on the LAN (would add login + port-based routing fallback, see ADR-0002).
- Internet exposure / HTTPS.
- Multiple users.
