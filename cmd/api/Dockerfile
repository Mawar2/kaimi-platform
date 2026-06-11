# Dockerfile for the Kaimi JSON API server (WS-B6).
#
# Build from the repository root (the build context must include go.mod and the
# config/ directory):
#
#   docker build -f cmd/api/Dockerfile -t kaimi-api .
#
# Unlike cmd/pipeline (a batch job that runs once and exits), this container is a
# long-lived HTTP server. Deploy it as a Cloud Run SERVICE: Cloud Run injects $PORT,
# which the server honors (PORT > API_PORT > default; see internal/httpapi/config.go),
# and the server binds 0.0.0.0 via API_HOST. It handles SIGTERM for graceful
# shutdown, so Cloud Run can drain it cleanly.
#
# Configuration is entirely via environment variables (see cmd/api/main.go and
# internal/httpapi/config.go): API_HOST, API_PORT/PORT, CORS_ALLOWED_ORIGINS, the
# OAUTH_*/SESSION_SECRET sign-in settings (from Secret Manager), plus the tenant
# config (GCP_PROJECT_ID, GCP_REGION, model names, ingest targets). The store path,
# profile path, and offline/live toggles are flags passed as container args.

# --- Build stage -------------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Download modules first so Docker layer caching skips re-downloading
# dependencies when only source files change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO disabled for a fully static binary that runs on distroless.
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

# --- Runtime stage -----------------------------------------------------------
# Distroless static: no shell, no package manager — minimal attack surface.
# The nonroot variant runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/api /app/api

# The API wires the same proposal service cmd/dashboard does, which reads the
# capability/eligibility profiles from disk. Bake the repo's config into the image;
# override paths with -profile / config env vars if needed.
COPY --from=build /src/config /app/config
COPY --from=build /src/test/fixtures /app/test/fixtures

ENTRYPOINT ["/app/api"]
