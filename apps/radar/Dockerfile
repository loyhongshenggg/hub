# syntax=docker/dockerfile:1
# Build context: the repo root (see compose.yaml).

# 1. Web UI → apps/radar/web/dist
FROM node:26-alpine AS web
RUN npm install -g pnpm@12.21.0
WORKDIR /repo
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.base.json ./
COPY libs/app-sdk/ libs/app-sdk/
COPY libs/ui-kit/ libs/ui-kit/
COPY apps/radar/web/ apps/radar/web/
RUN --mount=type=cache,id=hub-pnpm-store,target=/pnpm-store \
    pnpm install --frozen-lockfile --store-dir /pnpm-store --filter @hub/radar-web...
RUN pnpm --filter @hub/radar-web exec vite build

# 2. Go API with the UI embedded → /out/app
FROM golang:1.27-alpine AS api
WORKDIR /repo
COPY libs/go-appkit/ libs/go-appkit/
COPY apps/radar/api/ apps/radar/api/
WORKDIR /repo/apps/radar/api
COPY --from=web /repo/apps/radar/web/dist ui/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app . && mkdir -p /out/data

# 3. Small runtime image
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=api /out/app /app
# A named volume copies this dir's owner on first use, so nonroot can write.
COPY --from=api --chown=65532:65532 /out/data /data
ENV PORT=8080 DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/app"]
