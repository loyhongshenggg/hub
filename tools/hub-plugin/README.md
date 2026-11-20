# @hub/plugin

Local Nx plugin. Generator `app` creates `apps/<id>/` from templates derived from `apps/hello`:

```bash
pnpm nx g @hub/plugin:app <id> [--name=..] [--description=..] [--port=8080] [--healthPath=/healthz]
pnpm nx g @hub/plugin:app <id> --template=image --image=<ref> --port=<port> --healthPath=<path>
```

- Templates: `src/generators/app/files/{common,go-react,image}` (EJS; `.template` suffix is stripped). Use `<%- JSON.stringify(x) %>` for free text in YAML/Go/JSON/TSX.
- When `apps/hello` changes in a way new apps should get, update `files/go-react` too.
- Runs from source (no build step). Tests: `pnpm nx test @hub/plugin`.
