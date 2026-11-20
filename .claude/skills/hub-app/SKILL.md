---
name: hub-app
description: Create a new app for this hub platform, or plug an existing app or Docker image into it. Use when the user wants to add, scaffold, wrap, port, fix or remove an app under apps/ (app.manifest.yaml, compose.yaml, Dockerfile), or when `hub validate` fails.
---

# hub-app

An app = a web server in a container + a health endpoint + `apps/<id>/app.manifest.yaml`. Full guide (rules, manifest table, SDK, troubleshooting): `docs/adding-an-app.md`. Working reference: `apps/hello`.

## 1. Pick the path

| User has                            | Path                                                                 |
| ----------------------------------- | -------------------------------------------------------------------- |
| An idea, nothing built              | **New app** (generator, Go API + React UI)                           |
| A Docker image (own or third-party) | **Image** (generator, `--template=image`)                            |
| Code in another language/repo       | **Existing codebase** (hand-written Dockerfile + compose + manifest) |

Ask for the app id if it isn't obvious: lowercase, digits, single dashes (`notes`, `time-tracker`), ≤ 40 chars. It becomes the folder, `<id>.hub.localhost`, and compose project `hub-<id>`.

## 2. Do it

**New app**

```bash
pnpm nx g @hub/plugin:app <id> --description="<one line>" [--name="<Display Name>"] [--port=8080]
```

Then build the feature: API routes in `apps/<id>/api/main.go` (use `appkit` helpers; persist under `$DATA_DIR`), UI in `apps/<id>/web/src/app.tsx` (use `@hub/ui-kit` components so it looks native; `@hub/app-sdk` for badge/notify/openApp/setTitle). Add Go tests next to the code.

**Image**

Find from the image's docs/Dockerfile (search the web if needed): the HTTP port, a path answering 2xx **without login**, and the data directory. Then:

```bash
pnpm nx g @hub/plugin:app <id> --template=image --image=<ref:tag> --port=<port> --healthPath=<path>
```

Edit `apps/<id>/compose.yaml`: point the `data` volume at the image's data dir, add needed `environment:`, add companion services (db) if required. Pin the image tag; avoid `latest`.

**Existing codebase**

Copy the shape of `apps/hello` (or generate an image app and swap `image:` for `build:`). Write a multi-stage `Dockerfile` whose build context is the repo root (`context: ../..`), a `compose.yaml` with one routed service, the manifest, and a `project.json` with the `validate` target and tags `scope:app`, `app:<id>`.

## 3. Prove it

Run until it prints PASS; fix and repeat on failure (the output includes the last log lines):

```bash
pnpm nx run <id>:validate
```

For generated Go + React apps also run:

```bash
pnpm nx sync
pnpm nx run-many -t lint,test,typecheck,build -p <id> <id>-api @hub/<id>-web
pnpm nx run boundaries:lint
```

Then tell the user to open `http://hub.localhost:7000` and power the app on (it appears within ~2 s; no core restart).

## Rules (never break)

- One HTTP port, listening on `0.0.0.0`; `runtime.port` matches it. No `ports:` and no `container_name:` in compose.
- `health.path` returns 2xx when ready (redirects don't count). Raise `health.startTimeout` for slow starters.
- State only in named volumes.
- App code may import `libs/*` only — never another app or `platform/*`.
- Don't change `platform/` or `libs/manifest` to make one app work. If the contract truly lacks something, stop and propose a manifest change to the user (a breaking change bumps `manifestVersion` and keeps v1 working).
- Web `package.json` of an app lists every dependency its build needs (the Docker build installs only that package's deps). After changing deps run `pnpm install` at the repo root.

## Common validate failures

| Output                           | Fix                                                                        |
| -------------------------------- | -------------------------------------------------------------------------- |
| `connection refused`             | App binds `127.0.0.1` or another port → listen on `0.0.0.0:<runtime.port>` |
| `404` / `302` on health          | Wrong `health.path`, or it redirects to login → pick a 2xx path            |
| `not healthy after …`            | Slow boot → raise `startTimeout`; otherwise read the logs                  |
| `exited before becoming healthy` | Crash on boot → logs: missing env var, unwritable data dir, bad command    |
| build fails at `pnpm install`    | Lockfile out of date → `pnpm install` at root                              |

## Removing an app

Power it off, delete `apps/<id>/`, drop its line from `go.work`, run `pnpm install && pnpm nx sync`. Its data volume stays until `docker volume rm hub-<id>_data` (ask the user first — that deletes data).
