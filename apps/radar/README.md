# Radar

A learning dashboard built on curated tracks and mentors, plus SWE / security / AI news. Design: [docs/radar/design.md](../../docs/radar/design.md). Built in steps 3a–3f of Phase 2; 3a (content + tagging) and 3b (engine + API) are done, the learning UI comes in 3d.

- **Learning content** (`api/content/`, embedded into the binary, validated on start):
  - `skills.yaml`: 23 topics in 5 areas, with prerequisites and tagger keywords.
  - `mentors.yaml`: 5 mentors (with feeds) and gap-filler sources.
  - `tracks/*.yaml`: 3 full tracks (agentic engineering, AI red teaming, exploitation fundamentals) and 4 outlines.
- **Mentor feeds**: fetched with the news feeds. Their whole back catalogue is kept as learning material, never pruned. Items from the last 60 days are "fresh".
- **Tagging**: every item gets skill-map topics, difficulty, type and minutes from the rule-based tagger (`api/internal/tagger`). Edit keywords in `skills.yaml` and Radar retags on the next start. Hand fixes (`PUT /api/items/{id}/topics`) are never overwritten.
- **Engine** (`api/internal/engine`, pure, tested with simulated learners):
  - Within a chapter, steps keep their order. "Too easy" offers a skip to practice; "too hard" inserts an easier corrective step on a prerequisite topic.
  - Checkpoint: tick the outcomes you can do. All met → the chapter completes (mastery). Any unmet → another-angle step, or "continue anyway".
  - Next chapter: each track's frontier, scored by Thompson-sampled interest (topic + track), fit, learning progress and area priority, plus momentum when you like the current track. Unmet prerequisites lower the score.
  - Fresh mentor items show as optional steps in chapters on their topics. Weekly minutes goal in your timezone.
- **News**: 16 feeds plus recent mentor items, fetched every 30 minutes (and on Refresh) with conditional GETs. News shows the last 120 days. Filter by category or topic, search, unread / saved, mark read, save. Deep link: `#sources`.
- **Edit the news feeds** in [`api/content/sources.yaml`](api/content/sources.yaml). Removed feeds stop being fetched; saved items stay. Optional `topics:` sets fallback topics.

| Path                                | What                                                                                     |
| ----------------------------------- | ---------------------------------------------------------------------------------------- |
| `api/content/`                      | Curated data and its loader/validator (`learning.go`, `feeds.go`, `content.go`)          |
| `api/internal/fetch/`               | Feed fetching (gofeed), summary cleanup, pruning (unsaved news items > 120 days)         |
| `api/internal/engine/`              | Adaptive engine: dashboard, chapter/track views, ratings, checkpoints, correctives, week |
| `api/internal/tagger/`              | Rule-based topic tagger + the tagging pass (backfill / retag on rule changes)            |
| `api/internal/store/`               | SQLite in `$DATA_DIR/radar.db`: sources + fetch status, items, topic tags, read/saved    |
| `api/api.go`, `api/api_learning.go` | HTTP API (below)                                                                         |
| `web/src/`                          | React UI (`news.tsx`, `sources.tsx`) on `@hub/ui-kit`                                    |

API:

- `GET /api/curriculum`: areas, topics, mentors, gap fillers, tracks.
- `GET /api/learning`: dashboard (continue, up next, week, tracks, fresh count, needs assessment).
- `GET /api/learning/tracks/{track}`, `GET /api/learning/tracks/{track}/chapters/{chapter}`.
- `POST /api/learning/done` `{ref, rating?: easy|right|hard, worth?}`, `POST /api/learning/undo` `{ref}`, `POST /api/learning/dismiss` `{ref}`. Refs: `step:<track>/<chapter>/<step>`, `practice:<track>/<chapter>`, `capstone:<track>`, `item:<id>`.
- `POST …/chapters/{chapter}/skip-to-practice`, `…/checkpoint` `{met: [bool], rating?, worth?, force?}`, `…/snooze`.
- `GET|PUT /api/learning/assessment` `{answers: {topic: new|some|solid}}`, `GET|PUT /api/learning/settings` `{weeklyMinutes, priorities: {area: 0–3}, timezone}`.
- `GET /api/learning/history?limit=&before=`, `GET /api/learning/fresh`: recent mentor items (`mentor=`, `topic=`, `limit=`, `cursor=`).
- `GET /api/news`: filters `category`, `topic`, `mentor`, `q`, `unread=1`, `saved=1`, `limit`, `cursor`.
- `POST /api/news/read-all` (same filters).
- `POST /api/items/{id}/read`, `POST /api/items/{id}/saved`, `PUT /api/items/{id}/topics`.
- `GET /api/sources`, `GET /api/summary`, `POST /api/refresh`, `POST /api/visit`.

```bash
hub apps start radar --dev        # live reload inside the hub
pnpm nx run radar:validate        # Docker build + health check
```

Env: `RADAR_FETCH_INTERVAL` (default `30m`).
