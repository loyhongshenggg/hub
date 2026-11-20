# hello

Reference app: everything a hub app can use, in the smallest form. Copy it (or run the generator) to start a new app.

| Path | What |
|---|---|
| `app.manifest.yaml` | The contract the core reads (schema: `libs/manifest/schema/v1.schema.json`). |
| `compose.yaml` | The app's own services. One `web` service, a `data` volume, no host ports. |
| `Dockerfile` | Multi-stage, context = repo root: Vite build → Go build with the UI embedded → distroless image (~17 MB). |
| `api/` | Go module using `libs/go-appkit` (`/healthz`, config, SPA serving, graceful shutdown). |
| `web/` | React + Vite UI using `@hub/ui-kit` (looks native in the shell) and `@hub/app-sdk` (theme, badge, notify, openApp, title). |

```bash
pnpm nx run hello:validate        # build, start, health-check, tear down (throwaway instance)
pnpm nx run hello:docker-build    # build the image the way the core does

# UI work without Docker: API on :8080 + Vite on :5174 (proxies /api)
pnpm nx serve hello-api
pnpm nx dev @hub/hello-web
```

The web package lists its own dependencies so the Docker build installs only what it needs (`pnpm install --filter @hub/hello-web...`).
