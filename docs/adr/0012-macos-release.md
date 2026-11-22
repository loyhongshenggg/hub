# ADR-0012: macOS releases run natively, installed with Homebrew

- Status: Accepted (2026-11-22)

## Context

Finance should also install on a MacBook. macOS has no WSL, so the Windows `.wsl` file (ADR-0011) can't be reused. We wanted the simplest thing to build, and the simplest install for people who aren't developers.

## Options

| Option                                                       | Building it                                                                                       | Installing it                                               | Catch                                                                                                                                  |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| **Native + Homebrew**                                        | Cross-compile the hub and each app for darwin. Ship a formula.                                    | `brew tap` + `brew install`, no VM                          | Needs Homebrew. Apps run without container isolation, as in Dev mode.                                                                  |
| [Colima](https://github.com/abiosoft/colima) + Docker images | Same images as Windows, plus arm64 builds. Colima, Docker and Compose setup.                      | Homebrew plus four tools; a Linux VM starts on every launch | More moving parts; around 600 MB idle ([one report](https://dev.to/ashabhussan/switching-from-docker-desktop-to-colima-on-macos-1dm8)) |
| [Apple `container`](https://github.com/apple/container)      | A new hub driver; it has [no Compose support](https://github.com/apple/container/discussions/194) | A signed Apple installer                                    | macOS 26 and Apple Silicon only, and still young                                                                                       |
| Docker Desktop / [OrbStack](https://docs.orbstack.dev/faq)   | Same as Colima                                                                                    | A separate heavy app                                        | Paid licence for commercial use                                                                                                        |

## Decision

- **No Docker on the Mac.**
  - `hub serve --native` runs every app with the host-process driver (the Dev-mode driver, ADR-0008).
  - The release manifest's `dev:` block runs the app's prebuilt program, with the environment from its compose file. It's additive: no manifest version bump.
  - Shipped apps auto-start.
- **Builds.**
  - `tools/release/macos/build.sh` builds `<name>-macos-{arm64,amd64}.tar.gz` on Linux (`CGO_ENABLED=0`; SQLite is pure Go). Each tarball holds:
    - the hub;
    - each app's program, manifest and icon;
    - the launcher, the `<name>` command, and `<name>.icns`, drawn by `tools/desktop`.
  - `audit.sh` allows only the expected files.
- **Install.**
  - `Formula/<name>.rb` lives in this repo, which doubles as the Homebrew tap. CI regenerates it from the built tarballs.
  - Homebrew installs Poppler and Tesseract for statement OCR.
  - Formulas, unlike browser downloads, [aren't quarantined](https://thefridaydeploy.substack.com/p/my-struggles-with-gatekeeper-errors), so Gatekeeper needs no notarisation ($99/year). CI checks the extracted files carry no quarantine flag.
- **Opening it.**
  - The first `<name>` run creates `~/Applications/<Title>.app` on the Mac itself, so it isn't quarantined, and pins it to the Dock.
  - The app opens a Chrome or Edge app window. If the hub isn't running, it starts `hub serve --native --session` first, with a splash page while it starts.
  - Closing the window, or quitting the app from the Dock, stops the hub and its apps.
  - Data lives in `~/Library/Application Support/<Title>`.
- **Release.** The same tag builds Windows and Mac. A macOS runner installs Poppler and Tesseract, runs the arm64 tarball natively and checks every app starts, before anything is published.

## Consequences

- The Mac install is two Homebrew commands plus one `finance` run. Nothing to sign; no VM.
- The hub's process driver now builds on darwin. Pdeathsig is Linux-only, and stale-group cleanup does nothing outside Linux (no `/proc`).
- Apps run as the user, without container isolation. That's acceptable for a single-user local app, and Finance only reads the user's own statements.
- Apps that need extra system tools must list them as formula dependencies.
