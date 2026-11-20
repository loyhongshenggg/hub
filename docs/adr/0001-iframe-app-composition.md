# ADR-0001: Compose app UIs with iframes + a bridge SDK

- Status: Accepted (2026-11-20)

## Context

Apps are built over years, in whatever stack fits each one, and run in their own containers with their own backends. The shell must host them as tabs without coupling their build tooling or framework versions.

## Options

| Option | Finding |
|---|---|
| **iframe + postMessage bridge** | Strongest isolation; any stack per app. Lark web apps are standalone pages with a JS bridge injected by the client; Home Assistant shows add-on UIs in iframes. Weakness: modals are confined to the tab area. |
| Module Federation | Bundler-level code sharing, little isolation by default. Ties every app to the shell's bundler and shared deps — a shell React upgrade forces upgrading every app. |
| single-spa | Orchestration only; no sandbox, apps share the global scope. |
| wujie | iframe sandbox + renders app into the shell's DOM. No independent evidence on long-term maintenance. |

## Decision

Each app tab is an iframe pointing at the app's own origin. Optional `@hub/app-sdk` provides a typed, versioned postMessage bridge (context, theme, badge, notify, openApp). Running tabs stay mounted (hidden) to preserve state.

## Consequences

- Apps are fully independent: any framework, any version, deployable standalone.
- Shell upgrades never force app upgrades.
- Cross-app UI (global modals, drag between apps) must go through the bridge.

## Sources

- https://aiga.js.org/docs/comparison
- https://open.feishu.cn/llms.txt
- https://community.home-assistant.io/t/how-to-use-x-ingress-path-in-an-add-on/276905
