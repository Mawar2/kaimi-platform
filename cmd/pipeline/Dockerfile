# Dockerfile for the Kaimi Zone-1 pipeline runner (issue #122).
#
# Build from the repository root (the build context must include go.mod and
# the config/ directory):
#
#   docker build -f cmd/pipeline/Dockerfile -t kaimi-pipeline .
#
# The container runs cmd/pipeline once and exits — it is a batch job, not a
# server. Deploy it as a Cloud Run Job triggered by Cloud Scheduler.
# Configuration is entirely via environment variables (see cmd/pipeline/main.go):
# MODE, STORE_PATH, PROFILE_PATH, ELIGIBILITY_PROFILE_PATH, SAM_API_KEY,
# GCP_PROJECT_ID, GCP_REGION, GEMINI_MODEL.

# --- Build stage -------------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Download modules first so Docker layer caching skips re-downloading
# dependencies when only source files change.
COPY go.mod go.sum ./
# The telemetry core is an in-tree nested module consumed via a local replace
# (replace github.com/Mawar2/kaimi-telemetry => ./telemetry); its go.mod must exist
# before `go mod download` can resolve that replace.
COPY telemetry/go.mod ./telemetry/
RUN go mod download

COPY . .

# CGO disabled for a fully static binary that runs on distroless.
RUN CGO_ENABLED=0 go build -o /out/pipeline ./cmd/pipeline

# --- Runtime stage -----------------------------------------------------------
# Distroless static: no shell, no package manager — minimal attack surface.
# The nonroot variant runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/pipeline /app/pipeline

# The pipeline reads its capability/eligibility profiles from disk. Bake the
# repo's config into the image; the scoring-profile fixture is the current
# default profile until a production profile lands (see cmd/pipeline/main.go
# defaults). Override with PROFILE_PATH / ELIGIBILITY_PROFILE_PATH if needed.
COPY --from=build /src/config /app/config
COPY --from=build /src/test/fixtures /app/test/fixtures

ENTRYPOINT ["/app/pipeline"]
