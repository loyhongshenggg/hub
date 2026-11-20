# hub

Personal, locally hosted, Lark-style platform: one shell UI hosts many small apps; each app has a power button (starts/stops its containers) and opens in a tab.

- Architecture: [docs/architecture.md](docs/architecture.md)
- Decisions + sources: [docs/adr/](docs/adr/)

## Working rules

- Big architectural decisions (framework, runtime, integration model, infra): research current options online, compare, cite sources, then recommend. Small choices: use judgment.
- Present a design and get approval before building anything large.
- Apps never import other apps or `platform/*`; only `libs/*` (enforced by Nx tags).
- The app contract (`app.manifest.yaml`, `libs/manifest` JSON Schema) is versioned. Breaking changes bump `manifestVersion` and keep the old version working.

## Environment

- WSL2 Ubuntu 26.04, repo at `~/code/hub` (Linux filesystem — never move it under `/mnt/c`).
- Toolchain: Go 1.27, Node 26 LTS, pnpm 12, Docker Engine 29 + Compose v5 (no Docker Desktop). User `apple` is in the `docker` group.
- Re-provision: `sudo bash tools/setup/wsl-toolchain.sh apple <git-email>` (idempotent).
- Browser on Windows: `http://hub.localhost:7000` (Chrome/Edge/Firefox resolve `*.localhost`).
- Nx 24.0.6 workspace, created with `--preset=ts --pm=pnpm`, package scope `@hub`.
- Dev: `pnpm nx dev core` → shell (Vite + HMR) served through the core at `http://hub.localhost:7000`.
- Build: `pnpm nx build core` → `dist/platform/core/hub` with the shell embedded; run `dist/platform/core/hub serve`.
- Docker tests: `pnpm nx run core:integration`.

## Conventions

- Tags: every project has exactly one `scope:platform|app|lib|tool`; projects under `apps/<id>/` also get `app:<id>`. Rules live in [tools/boundaries/constraints.mjs](tools/boundaries/constraints.mjs), used by ESLint (TS imports) and `pnpm nx run boundaries:lint` (whole graph, incl. Go imports).
- Go modules: path `hub.localhost/<repo-path>` (reserved TLD, never fetched). Add each to `go.work`. A module that imports another workspace module also gets `require <path> v0.0.0` + `replace <path> => ../..` so `go mod tidy` and Docker builds work without `go.work`.
- Go projects need a `project.json` with `name` + `tags` (gonx ignores `--tags`). Go `lint` = `go vet` (nx.json targetDefaults).
- One language per Nx project root (Go and TS targets share names). Mixed libs nest the TS package: `libs/manifest` (Go) + `libs/manifest/ts` (`@hub/manifest`).
- Shell/ui-kit: React 19 + Vite 8 + Tailwind v4 + Radix; TanStack Query (server state, kept live by `/api/events` SSE) + Zustand (UI state). ui-kit components are hand-written in shadcn/ui style; `@hub/ui-kit/theme.css` holds the tokens and `@source`s itself for consumers.
- App layout (see `apps/hello`): `app.manifest.yaml`, `compose.yaml`, `Dockerfile` (build context = repo root; `.dockerignore` at root), `api/` Go module embedding `web/` build via `api/ui/dist`, `web/` React app whose `package.json` lists its own deps (Docker installs with `--filter <web>...`). Tags: `scope:app`, `app:<id>` on all three projects.
- ESLint 9, not 10: eslint-plugin-react / jsx-a11y / import don't support 10 yet (checked 2026-11-20). ESLint 9 is marked deprecated upstream — move to 10 once those plugins support it.
- After adding/removing a TS project or workspace dep: `pnpm install && pnpm nx sync` (TS project references), or Nx tasks stop with "workspace is out of sync".
- Manifest schema: `libs/manifest/schema/v<N>.schema.json` is the source of truth. TS types are generated (`codegen` target, runs before build/typecheck); Go types are hand-written and a test fails if they drift.

## Status

- [x] Phase 0 — environment (2026-11-20)
- [x] Phase 1 — foundations (2026-11-20)
  - [x] Nx 24 workspace created (ts preset, pnpm)
  - [x] Workspace layout: `platform/`, `apps/`, `libs/`, `tools/` in pnpm-workspace.yaml; drop `packages/`
  - [x] Go: `go.work`, `@naxodev/gonx` 5.x, ESLint + `@nx/enforce-module-boundaries` tags
  - [x] libs/manifest — JSON Schema + Go/TS types
  - [x] platform/core — registry, docker driver, lifecycle, gateway, SSE, SQLite, `hub` CLI + `validate`
  - [x] platform/shell — dock, home grid, power toggles, keep-alive tabs, logs drawer, light/dark
  - [x] libs/ui-kit — tokens + shadcn-style components (pulled forward for the shell)
  - [x] libs/app-sdk, libs/go-appkit
  - [x] apps/hello — reference app
  - [x] tools/hub-plugin — `nx g @hub/plugin:app <id>` (go-react + image templates)
  - [x] .claude/skills/hub-app — create / plug in an app
  - [x] docs/adding-an-app.md
- [ ] Phase 2 — dev ergonomics
- [ ] Phase 3 — platform services

<!-- nx configuration start-->
<!-- Leave the start & end comments to automatically receive updates. -->

# General Guidelines for working with Nx

- For navigating/exploring the workspace, invoke the `nx-workspace` skill first - it has patterns for querying projects, targets, and dependencies
- When running tasks (for example build, lint, test, e2e, etc.), always prefer running the task through `nx` (i.e. `nx run`, `nx run-many`, `nx affected`) instead of using the underlying tooling directly
- Prefix nx commands with the workspace's package manager (e.g., `pnpm nx build`, `npm exec nx test`) - avoids using globally installed CLI
- You have access to the Nx MCP server and its tools, use them to help the user
- For Nx plugin best practices, check `node_modules/@nx/<plugin>/PLUGIN.md`. Not all plugins have this file - proceed without it if unavailable.
- NEVER guess CLI flags - always check nx_docs or `--help` first when unsure

## Scaffolding & Generators

- For scaffolding tasks (creating apps, libs, project structure, setup), ALWAYS invoke the `nx-generate` skill FIRST before exploring or calling MCP tools

## When to use nx_docs

- USE for: advanced config options, unfamiliar flags, migration guides, plugin configuration, edge cases
- DON'T USE for: basic generator syntax (`nx g @nx/react:app`), standard commands, things you already know
- The `nx-generate` skill handles generator discovery internally - don't call nx_docs just to look up generator syntax

<!-- nx configuration end-->
