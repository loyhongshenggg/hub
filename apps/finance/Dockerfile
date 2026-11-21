# syntax=docker/dockerfile:1
# Build context: the repo root (see compose.yaml).

# 1. Web UI → apps/finance/web/dist
FROM node:26-alpine AS web
RUN npm install -g pnpm@12.21.0
WORKDIR /repo
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.base.json ./
COPY libs/app-sdk/ libs/app-sdk/
COPY libs/ui-kit/ libs/ui-kit/
COPY apps/finance/web/ apps/finance/web/
RUN --mount=type=cache,id=hub-pnpm-store,target=/pnpm-store \
    pnpm install --frozen-lockfile --store-dir /pnpm-store --filter @hub/finance-web...
RUN pnpm --filter @hub/finance-web exec vite build

# 2. Go API with the UI embedded → /out/app
FROM golang:1.27-alpine AS api
WORKDIR /repo
COPY libs/go-appkit/ libs/go-appkit/
COPY apps/finance/api/ apps/finance/api/
WORKDIR /repo/apps/finance/api
COPY --from=web /repo/apps/finance/web/dist ui/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app . && mkdir -p /out/data

# 3. Runtime: Debian slim, because statement PDFs (HSBC SG has no text
# layer) are rendered with poppler and read with Tesseract OCR.
FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils tesseract-ocr tesseract-ocr-eng ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 65532 --user-group --no-create-home --shell /usr/sbin/nologin nonroot
COPY --from=api /out/app /app
# A named volume copies this dir's owner on first use, so nonroot can write.
COPY --from=api --chown=65532:65532 /out/data /data
USER 65532:65532
# One OpenMP thread per Tesseract process: pages are read in parallel.
ENV PORT=8080 DATA_DIR=/data OMP_THREAD_LIMIT=1
EXPOSE 8080
ENTRYPOINT ["/app"]
