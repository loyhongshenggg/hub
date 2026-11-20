# ADR-0003: Develop inside WSL2 Ubuntu on the Linux filesystem

- Status: Accepted (2026-11-20)

## Context

Host is Windows 11 Home. The original working folder was inside OneDrive, which syncs `node_modules`, Nx cache and build output (slow installs, EPERM file locks, flaky Docker builds). Apps run in Linux containers.

## Options

| Option | Finding |
|---|---|
| Windows native in OneDrive | Sync conflicts and locks — rejected. |
| Windows native, non-OneDrive folder | Works; NTFS is slow at many small files; Docker crosses a VM boundary. |
| **WSL2 Ubuntu, repo on ext4** | Consistent recommendation for Node/Go/Docker on Windows; avoids the 9P cross-filesystem penalty; Linux matches containers. Windows browsers reach WSL servers at `localhost` by default. |

## Decision

Repo lives at `~/code/hub` inside WSL2 Ubuntu 26.04. Edit with VS Code + WSL extension. Default networking (NAT with localhost forwarding).

## Consequences

- Fast installs/builds/watchers; same OS as containers.
- Claude Code config inside WSL (`~/.claude`) is separate from Windows'.
- WSL idles out when no process keeps it alive; the hub core is that process (start-on-login is Phase 2).

## Sources

- https://kx.cloudingenium.com/en/wsl2-windows-linux-development-environment/
- https://markaicode.com/wsl-vs-native-linux-performance-comparison/
- https://learn.microsoft.com/en-us/windows/wsl/networking
- https://learn.microsoft.com/en-us/answers/questions/1192206/wsl2-cant-run-in-the-background-after-enabling-sys
