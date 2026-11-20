# ADR-0002: Address apps by subdomain under `hub.localhost`

- Status: Accepted (2026-11-20)

## Context

Each app's UI is loaded in an iframe through the core's gateway. Existing web apps should plug in unchanged, logins must work inside the tab, and apps must not be able to touch the shell or each other.

## Options

| Option | Finding |
|---|---|
| Path `/apps/notes/` | Home Assistant's ingress uses this; long-running breakage of assets and SPAs that expect `/`. Same origin → no isolation. |
| Port `localhost:7101` (Umbrel/CasaOS style) | Apps at `/`, works by IP on the LAN. Cookies are not isolated by port → two apps' `session` cookies overwrite each other. |
| **Subdomain `notes.hub.localhost:7000`** | Chrome and Firefox resolve `*.localhost` to loopback with no hosts file. Apps at `/`. |

Same-site check (from Chromium source): `GetDomainAndRegistry` uses `INCLUDE_UNKNOWN_REGISTRIES`, so `localhost` acts as the TLD. `notes.hub.localhost` and `hub.localhost` both map to site `hub.localhost` → same-site, cross-origin. Bare `localhost` has no registrable domain, so `localhost` vs `notes.localhost` would be cross-site and Lax cookies would not be sent inside the iframe.

## Decision

- Shell at `http://hub.localhost:7000`; `http://localhost:7000` redirects there.
- App `<id>` at `http://<id>.hub.localhost:7000`, routed by Host header.
- Port-based routing is the documented fallback if LAN access is ever needed (with login).

## Consequences

- Existing apps work unmodified at `/`; cookies are per-app (host-only).
- Not reachable by IP from other devices without extra DNS — accepted (this PC only).
- Safari/system resolvers may not resolve `*.localhost`; use Chrome/Edge/Firefox.

## Sources

- https://raw.githubusercontent.com/chromium/chromium/main/net/base/registry_controlled_domains/registry_controlled_domain.cc
- https://raw.githubusercontent.com/chromium/chromium/main/net/base/schemeful_site.cc
- https://dev.to/k4ml/firefox-and-chrome-resolve-any-localhost-domain-localhost-to-loopback-address-26d9
- https://community.home-assistant.io/t/how-to-handle-absolute-paths-with-ha-ingress/370572
- https://jguillaumesio.com/blog/cookies-across-subdomains-localhost/
