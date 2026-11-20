# ADR-0004: Run Docker Engine inside the Ubuntu distro

- Status: Accepted (2026-11-20)

## Context

Apps run as containers. The core (in WSL Ubuntu) must control them and route traffic to them.

## Options

| Option | Finding |
|---|---|
| Docker Desktop | Most polished GUI; free for personal use; runs its own VM distro; heavier. |
| Podman Desktop | Trails Docker Desktop on polish; more setup on Windows. |
| Rancher Desktop | Kubernetes-oriented; not needed. |
| **Docker Engine in Ubuntu** | Open source, free in every case, no extra VM. Core talks to `/var/run/docker.sock` and reaches containers by IP on a private network. |

## Decision

Install `docker-ce` + `docker-compose-plugin` + `docker-buildx-plugin` from Docker's apt repo inside Ubuntu; systemd manages the daemon. The shell is the GUI.

## Consequences

- Gateway proxies straight to container IPs on the `hub` network; no host-port management.
- No Docker Desktop dashboard; `docker` CLI and the hub shell cover it.
- The runtime driver interface keeps Docker Desktop/Podman possible later.

## Sources

- https://docs.docker.com/subscription/desktop-license/
- https://tech-insider.org/sv/docker-desktop-vs-podman-vs-rancher-desktop-2026/
- https://docs.docker.com/engine/install/ubuntu/
