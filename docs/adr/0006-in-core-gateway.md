# ADR-0006: Build the gateway into the Go core

- Status: Accepted (2026-11-20)

## Context

Browser traffic for `<id>.hub.localhost` must reach the right container, and only while the app is running.

## Options

| Option | Finding |
|---|---|
| Traefik with Docker labels | Runtipi's choice — they are internet-facing and need auto-HTTPS and forward-auth. Adds a second routing brain to keep in sync with core lifecycle state. |
| Caddy (embedded or sidecar) | Same sync problem; HTTPS not needed locally. |
| **`httputil.ReverseProxy` inside the core** | One source of truth; WebSockets supported; full control of headers. |

## Decision

The core routes by Host header to the app's container IP:port from lifecycle state. When the app is not running it serves an "app is off" page. It strips `X-Frame-Options` and CSP `frame-ancestors` from app responses so the shell can frame them (a common Home Assistant add-on failure).

## Consequences

- No extra container; routing can never disagree with lifecycle state.
- If the hub is ever exposed to the internet, revisit (Traefik/Caddy for TLS).

## Sources

- https://runtipi.io/docs/reference/architecture
- https://archy.net/home-assistant-haproxy-fixing-add-on-access-blocked-by-x-frame-options/
