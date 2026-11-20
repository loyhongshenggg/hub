# hub

A personal, locally hosted, Lark-style platform: one shell UI hosts many small apps. Each app has a power button (starts/stops its containers) and opens in a tab.

```
http://hub.localhost:7000          the shell: dock, home grid, tabs, logs
http://<id>.hub.localhost:7000     each app, on its own origin (framed by the shell)
```

## Run it

```bash
pnpm install
pnpm nx build core                 # Go core with the shell embedded → dist/platform/core/hub
dist/platform/core/hub serve       # then open http://hub.localhost:7000
```

Apps keep running when the core stops and are picked up again on the next start.

## Develop

```bash
pnpm nx dev core                               # shell with hot reload, served through the core
pnpm nx run-many -t lint,test,typecheck,build  # everything (Go + TS)
pnpm nx run core:integration                   # Docker driver tests against the real engine
```

CLI (talks to the running core): `hub apps ls | start | stop | restart | logs <id>`, `hub validate <id>`.

## Add an app

```bash
pnpm nx g @hub/plugin:app <id>                    # Go API + React UI
pnpm nx g @hub/plugin:app <id> --template=image --image=<ref> --port=<port> --healthPath=<path>
pnpm nx run <id>:validate
```

Guide: [docs/adding-an-app.md](docs/adding-an-app.md). With Claude Code, the `hub-app` skill does this for you.

## Layout

| Path               | What                                                                                            |
| ------------------ | ----------------------------------------------------------------------------------------------- |
| `platform/core`    | Go: registry, Docker driver, lifecycle, gateway, API, SQLite, `hub` CLI                         |
| `platform/shell`   | React shell UI (embedded into the core binary)                                                  |
| `apps/<id>`        | One folder per app; `apps/hello` is the reference                                               |
| `libs/manifest`    | App manifest JSON Schema + Go/TS types                                                          |
| `libs/go-appkit`   | Optional Go kit for apps (health, config, SPA, shutdown)                                        |
| `libs/app-sdk`     | Optional shell ⇄ app bridge (`@hub/app-sdk`)                                                    |
| `libs/ui-kit`      | Shared React components + design tokens (`@hub/ui-kit`)                                         |
| `tools/hub-plugin` | `nx g @hub/plugin:app`                                                                          |
| `docs/`            | [architecture](docs/architecture.md), [ADRs](docs/adr/), [adding an app](docs/adding-an-app.md) |
