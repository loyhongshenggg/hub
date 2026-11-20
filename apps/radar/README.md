# Radar

Live SWE / security / AI news (step 2 of Phase 2) — mentors and a learning path come next.

- **News**: 16 feeds, fetched every 30 minutes (and on Refresh) with conditional GETs. Filter by category, search, unread / saved, mark read, save for later. Deep link: `#sources`.
- **Sources**: each feed's item count, last success and last error.
- **Edit the feeds** in [`api/content/sources.yaml`](api/content/sources.yaml) (embedded into the binary; removed feeds stop being fetched, saved items stay).

| Path                  | What                                                                        |
| --------------------- | --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `api/content/`        | Curated, version-controlled data (`sources.yaml`)                           |
| `api/internal/fetch/` | Feed fetching (gofeed), summary cleanup, pruning (unsaved items > 120 days) |
| `api/internal/store/` | SQLite in `$DATA_DIR/radar.db`: sources + fetch status, items, read/saved   |
| `api/api.go`          | `GET /api/news`, `POST /api/items/{id}/read                                 | saved`, `POST /api/news/read-all`, `GET /api/sources`, `GET /api/summary`, `POST /api/refresh`, `POST /api/visit` |
| `web/src/`            | React UI (`news.tsx`, `sources.tsx`) on `@hub/ui-kit`                       |

```bash
hub apps start radar --dev        # live reload inside the hub
pnpm nx run radar:validate        # Docker build + health check
```

Env: `RADAR_FETCH_INTERVAL` (default `30m`).
