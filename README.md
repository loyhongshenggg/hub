# hub

A personal, locally hosted, Lark-style platform: one shell UI hosts many small apps. Each app has a power button (starts/stops its containers) and opens in a tab.

```
http://hub.localhost:7000          the shell: dock, home grid, tabs, logs
http://<id>.hub.localhost:7000     each app, on its own origin (framed by the shell)
```

## Install on Windows

**Finance** (goals, spending insights and credit-card statement import for Singapore) is released as a Windows download. It runs entirely on your PC, inside its own small Linux system (WSL). Your data never leaves your machine.

**You need:**

- Windows 11, or Windows 10 22H2
- about 3 GB of free disk
- Google Chrome or Microsoft Edge

**Steps:**

1. **Turn on WSL (once).** Open **PowerShell as administrator** and run:

   ```powershell
   wsl --install --no-distribution
   ```

   Restart when Windows asks. If you already use WSL, run `wsl --update` instead. Finance needs WSL 2.4.8 or newer; check with `wsl --version`.

2. **Download `finance.wsl`** from the [latest release](../../releases/latest).
3. **Double-click `finance.wsl`.** Windows installs it, and a short setup window ends with _"Finance … is installed"_. Close that window.
4. **Open Finance** with the new **Finance** icon on your Desktop. The first start takes about a minute.

Closing the Finance window stops it completely. Opening the icon starts it again where you left off.

| To                                                                                                                                  | Do (in PowerShell)                                                                                               |
| ----------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Update to the newest version                                                                                                        | `wsl -d finance -e /opt/hub/update.sh`. Your data is kept. Close Finance first.                                  |
| Check the download is genuine (every release is built by GitHub Actions from a fresh checkout and audited to hold no personal data) | Compare with `SHA256SUMS` on the release page, or run the `gh attestation verify` command from the release notes |
| Fix a missing Desktop icon                                                                                                          | `wsl -d finance -u root -e /opt/hub/first-run.sh`                                                                |
| See what went wrong                                                                                                                 | Read `%LOCALAPPDATA%\finance\core.log` and `launcher.log`                                                        |
| Uninstall (**deletes your Finance data**)                                                                                           | `wsl --unregister finance`, then delete the Desktop icon and `%LOCALAPPDATA%\finance`                            |

Claude features in Finance are optional. They need your own Anthropic API key (Finance → Settings) and have a monthly spending cap.

## Run it (developers)

```bash
pnpm install
pnpm nx build core                 # Go core with the shell embedded → dist/platform/core/hub
dist/platform/core/hub serve       # then open http://hub.localhost:7000
```

Apps keep running when the core stops and are picked up again on the next start. `hub stop` stops a running core.

### Releasing

A release is a WSL distribution file with the hub and a chosen set of apps ([ADR-0011](docs/adr/0011-releases.md)). `releases/<name>.env` lists what ships (Finance: `releases/finance.env`).

```bash
RELEASE_REPO=<owner>/hs-hub tools/release/build.sh finance 0.1.0   # local build → dist/release/finance/
git tag finance-v0.1.0 && git push origin finance-v0.1.0           # GitHub Actions builds and publishes it
```

### Desktop icon (Windows)

```bash
pnpm nx run desktop:install        # puts a "hub" shortcut on the Windows Desktop
```

Click it to open the hub in its own window. It starts the hub if it isn't running, and when you close the window it stops the hub and its apps, which come back on the next launch. Logs: `%LOCALAPPDATA%\hub`. Details: [ADR-0009](docs/adr/0009-desktop-launcher.md).

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
