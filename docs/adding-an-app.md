# Adding an app

An app is a web server in a container, with a health endpoint and an `app.manifest.yaml`. That's the whole contract. Everything else (Go kit, UI kit, SDK) is optional help.

Three ways in:

| You have…                             | Do this                                                                                                 | Time      |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------- | --------- |
| Nothing yet                           | `pnpm nx g @hub/plugin:app <id>`: Go API + React UI, like `apps/hello`                                  | 2 min     |
| A Docker image (yours or third-party) | `pnpm nx g @hub/plugin:app <id> --template=image --image=<ref> --port=<port> --healthPath=<path>`       | 5 min     |
| An existing codebase (any language)   | Write a `Dockerfile`, `compose.yaml` and manifest by hand (see [Existing codebase](#existing-codebase)) | 15–30 min |

Every path ends the same way:

```bash
pnpm nx run <id>:validate     # build, start, wait for health, check the UI, tear down
```

Then open `http://hub.localhost:7000` and turn the app on. New folders under `apps/` show up within ~2 seconds; no core restart needed.

## New app

```bash
pnpm nx g @hub/plugin:app time-tracker --description="Track where the day went"
pnpm nx run time-tracker:validate
```

Options: `--name` (display name, default: the id in Title Case), `--port` (default 8080), `--healthPath` (default `/healthz`), `--dry-run`.

You get:

```
apps/time-tracker/
├─ app.manifest.yaml   the contract the core reads
├─ compose.yaml        one `web` service + a `data` volume
├─ Dockerfile          Vite build → Go build with the UI embedded → distroless image
├─ project.json        Nx targets: validate, docker-build
├─ api/                Go (libs/go-appkit): main.go has GET /api/hello; data goes in $DATA_DIR
└─ web/                React + Vite (@hub/ui-kit, @hub/app-sdk): start in src/app.tsx
```

Day-to-day work: turn the app on in **Dev mode** (app menu → _Start in dev mode_, or `hub apps start <id> --dev`). The core runs the API and the Vite dev server on your machine and routes the app's URL to them, so you work inside the shell with:

- UI edits: instant (hot reload).
- Go edits: the API restarts in about a second. A compile error waits for your fix and is shown in **Logs**.
- Data: a separate dev folder (`~/.local/share/hub/dev/<id>/data`), never the Run-mode volume.

Run mode (the power switch) uses the Docker image; rebuild it after changes with `pnpm nx run <id>:docker-build` and **Restart**.

## Existing Docker image

```bash
pnpm nx g @hub/plugin:app uptime --template=image \
  --image=louislam/uptime-kuma:2 --port=3001 --healthPath=/
```

From the image's docs, find:

1. **Port**: the HTTP port inside the container. It must listen on `0.0.0.0`, not `127.0.0.1`.
2. **Health path**: any path that answers **2xx without logging in**. A redirect to `/login` is not 2xx; try `/health`, `/api/health`, `/ping`, or a static asset like `/favicon.ico`.
3. **Data directory**: point the `data` volume in `compose.yaml` at it.
4. **Env vars**: add them under `environment:`.

Extra services (a database, a worker) go in the same `compose.yaml`. They reach each other by service name; only the service named in `runtime.service` is routed by the hub.

## Existing codebase

Copy `apps/hello` as a starting point, or write the three files yourself:

1. **`Dockerfile`**: builds the app into an image that serves HTTP on one port. Build context is the repo root (`context: ../..` in compose), so paths in `COPY` start from the repo root. Keep it multi-stage and small.
2. **`compose.yaml`**: the app's services. Name the routed one (`web` by convention). No `ports:`, no `container_name:`.
3. **`app.manifest.yaml`**: see [Manifest](#manifest). The folder name must equal `id`.
4. **`project.json`** (optional but recommended): copy the `validate` target from `apps/hello/project.json`, and tag the project `scope:app` + `app:<id>`.

If the code lives in this repo, it may use `libs/*` but never another app or `platform/*` (`pnpm nx run boundaries:lint` enforces this for TS and Go).

## The rules

1. One HTTP port inside the container, listening on `0.0.0.0`. No host ports.
2. `GET <health.path>` returns 2xx when ready.
3. The UI is served at `ui.path` (default `/`) as a normal page. Absolute paths like `/assets/app.js` are fine: each app has its own origin (`http://<id>.hub.localhost:7000`).
4. State lives in named volumes. Power off runs `docker compose down`, which keeps volumes.
5. No `container_name` (it clashes with `hub validate`'s throwaway copy).
6. Don't link to `localhost:<port>` in the UI; use relative URLs. The browser only sees the gateway.

The hub injects into the routed service: `HUB_APP_ID`, `HUB_APP_URL` (browser URL of the app), `HUB_CORE_URL` (browser URL of the shell), and labels `hub.app.id`, `hub.managed`. Apps run as compose project `hub-<id>` on the shared `hub` network.

The gateway removes `X-Frame-Options` and CSP `frame-ancestors` from responses so the shell can frame the app, and allows only the shell to frame it.

## Manifest

Schema: [`libs/manifest/schema/v1.schema.json`](../libs/manifest/schema/v1.schema.json). VS Code autocompletes it with the YAML extension.

| Field                 | Required     | Default          | Notes                                                                                                                                                                                   |
| --------------------- | ------------ | ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `manifestVersion`     | yes          |                  | `1`                                                                                                                                                                                     |
| `id`                  | yes          |                  | `[a-z][a-z0-9]*(-[a-z0-9]+)*`, ≤ 40 chars, equals the folder name                                                                                                                       |
| `name`                | yes          |                  | Dock and tab label, ≤ 40 chars                                                                                                                                                          |
| `description`         |              | `""`             | Home grid, ≤ 200 chars                                                                                                                                                                  |
| `icon`                |              | letter tile      | `.svg` or `.png`, relative to the manifest                                                                                                                                              |
| `runtime.driver`      | yes          |                  | `docker`                                                                                                                                                                                |
| `runtime.compose`     |              | `./compose.yaml` | Relative, inside the app folder                                                                                                                                                         |
| `runtime.service`     | yes          |                  | Compose service the gateway routes to                                                                                                                                                   |
| `runtime.port`        | yes          |                  | Port inside that container                                                                                                                                                              |
| `health.path`         | yes          |                  | Must start with `/`                                                                                                                                                                     |
| `health.startTimeout` |              | `60s`            | Time allowed for the first 2xx after containers start (`ms`, `s`, `m`)                                                                                                                  |
| `ui.path`             |              | `/`              | Page opened in the tab                                                                                                                                                                  |
| `ui.keepAlive`        |              | `true`           | Keep the iframe loaded when switching tabs                                                                                                                                              |
| `autoStart`           |              | `false`          | Start with the hub (users can override per app in the UI)                                                                                                                               |
| `dev.route`           |              | first process    | Process the gateway routes to in Dev mode                                                                                                                                               |
| `dev.processes[]`     | for Dev mode |                  | `name`, `command` (`/bin/sh -c`), optional `dir` and `watch` (file patterns that restart it). Each gets `$PORT`; siblings' ports are in `$HUB_PORT_<NAME>`; data dir in `$HUB_DATA_DIR` |

Invalid manifests appear on the Home page with the exact problem; `hub validate <id>` prints the same.

## Talking to the shell (optional)

```ts
import { connect, syncThemeClass } from '@hub/app-sdk';

const hub = connect(); // safe outside the hub too
syncThemeClass(hub); // <html class="dark"> follows the hub theme (fits @hub/ui-kit)

hub.onContext((c) => console.log(c.appId, c.locale));
hub.setBadge(3); // dock + tab badge; 0 clears
hub.notify('Saved', 'All changes stored');
hub.openApp('notes', '/today'); // path is used if that tab isn't open yet
hub.setTitle('Inbox (3)'); // tab title; null restores the name
```

No JavaScript bundler? The protocol is plain `postMessage`. Send to `window.parent` with target origin = your origin minus the first label (`http://notes.hub.localhost:7000` → `http://hub.localhost:7000`):

| Direction   | Message                                                                                                                                                                                  |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| app → shell | `{hub: 1, type: "ready"}`, `{hub: 1, type: "setBadge", count}`, `{hub: 1, type: "notify", title, body?}`, `{hub: 1, type: "openApp", appId, path?}`, `{hub: 1, type: "setTitle", title}` |
| shell → app | `{hub: 1, type: "context", context: {appId, theme, locale}}` (answer to `ready`), `{hub: 1, type: "theme", theme}`                                                                       |

## Troubleshooting

`hub validate <id>` prints the last 40 log lines on failure. In the hub: **Logs** on the app card.

| Symptom                                        | Likely cause                                                  | Fix                                                                 |
| ---------------------------------------------- | ------------------------------------------------------------- | ------------------------------------------------------------------- |
| App missing from Home                          | Manifest invalid, or `id` ≠ folder name                       | Read the red box on Home, or run `hub validate <id>`                |
| `GET /healthz: … connection refused`           | App listens on `127.0.0.1`, or on another port                | Listen on `0.0.0.0:<runtime.port>`                                  |
| `GET /healthz: 404`                            | Wrong `health.path`                                           | Point it at a path that exists                                      |
| `GET /…: 302 Found`                            | Health path redirects (often to login)                        | Use a path that answers 2xx without auth                            |
| `not healthy after 60s`                        | Slow first start (migrations, JVM…)                           | Raise `health.startTimeout`                                         |
| `service "web" exited before becoming healthy` | Crash on boot                                                 | Read the logs; often a missing env var or a data dir it can't write |
| `docker … build` fails at `pnpm install`       | Web deps changed but lockfile not updated                     | Run `pnpm install` at the repo root and commit `pnpm-lock.yaml`     |
| Tab is blank but the app works at its own URL  | Frame-busting JS, or links to `localhost:<port>`              | Remove frame-busting; use relative URLs                             |
| `421 unknown host`                             | Opened via `127.0.0.1` or a custom host                       | Use `http://hub.localhost:7000` / `http://<id>.hub.localhost:7000`  |
| Code changes don't show in the hub             | Image not rebuilt                                             | `pnpm nx run <id>:docker-build`, then Restart                       |
| Data gone                                      | Volume removed (`down -v`) or data written outside the volume | Check the volume path in `compose.yaml`                             |

## Removing an app

1. Power it off in the hub.
2. Delete `apps/<id>/`. Remove its line from `go.work` (Go apps), then run `pnpm install` and `pnpm nx sync`.
3. Optional, deletes its data: `docker volume rm hub-<id>_data` (`docker volume ls | grep hub-<id>` to see them).
