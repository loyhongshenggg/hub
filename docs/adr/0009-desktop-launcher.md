# ADR-0009: A Desktop icon that starts the hub and stops it on close

- Status: Accepted (2026-11-21)

## Context

The user wants to click a Desktop icon to open the hub. If the hub isn't running, the click should start it. When the user closes the window, the hub and its apps should stop. The hub runs inside WSL; Docker Engine runs there too (ADR-0003, ADR-0004).

Two facts shape the design:

- WSL keeps a distro up while a process started through `wsl.exe` is still running. Background services don't keep it up ([Microsoft: systemd in WSL](https://learn.microsoft.com/en-us/windows/wsl/systemd), [WSL timers discussion](https://github.com/microsoft/WSL/discussions/8659?sort=new)).
- This machine blocks unsigned programs ("An Application Control policy has blocked this file"), even with Smart App Control off. So a launcher `.exe` built locally won't run.

## Options

| Option                                              | Starts the hub                 | Stops it on close | Runs here                                |
| --------------------------------------------------- | ------------------------------ | ----------------- | ---------------------------------------- |
| Install the shell as a web app from the browser     | No: a web page can't start WSL | No                | Yes                                      |
| Small launcher `.exe` (Go, `-H windowsgui`)         | Yes                            | Yes               | **No: unsigned, blocked by App Control** |
| **Shortcut → signed Windows tools → script in WSL** | Yes                            | Yes               | Yes                                      |
| Native wrapper (Tauri / WebView2)                   | Yes                            | Yes               | Needs code signing too                   |

## Decision

- **Shortcut.** `Desktop\hub.lnk` runs `conhost.exe --headless wsl.exe -d <distro> --cd <repo> -e bash tools/desktop/launch.sh`. Every Windows binary in that chain is signed (conhost, wsl, chrome), and headless conhost shows no console window.
- **`launch.sh`, when the hub already answers** (a terminal, start-on-login, a second click): it opens a hub window and stops nothing.
- **`launch.sh`, when it doesn't:**
  1. Wait for Docker.
  2. Run `hub serve --session`.
  3. Open a Chrome app window (`--app`, its own profile, so it is a separate process the script can wait on) on a "Starting hub…" page that switches to the hub when it answers.
  4. When the window closes, run `hub stop` and wait for the core to exit. WSL then idles down.
- **`hub serve --session`.** On exit it records the apps that are running (with their mode), stops them, and then exits. The next start, with or without `--session`, brings those apps back. Without `--session`, shutdown is unchanged: Run-mode apps keep running and are re-adopted.
- **`hub stop` / `POST /api/system/shutdown`.** The server's same-origin check guards this endpoint like every other state-changing call, so other websites and app iframes can't stop the hub.
- **Icon.** `tools/desktop` (Go) draws the hub logo into `hub.ico` (PNG entries, 16–256 px). There are no image tools to install.
- **Install.** `pnpm nx run desktop:install` builds the core and the icon, writes `~/.config/hub/desktop.env` (Chrome path, state folder) and creates the shortcut. It's safe to re-run.

"Start on Windows login" stays planned as ADR-0010. When the hub is already up, the icon simply opens a window.

## Consequences

- One click opens the hub, and closing the window turns everything off: apps, dev processes, the core and, after its idle timeout, WSL.
- Logs live in `%LOCALAPPDATA%\hub` (`launcher.log`, `core.log`).
- The launcher depends on Chrome being installed. The window has its own Chrome profile, separate from the user's.
- Shipping to other machines would still need signing for any native binary (see the release proposal).
