# kaimi-telemetry (working name)

The privacy-first, self-hosted **agent + product observability** core.

It runs *inside* the host application's deployment. A redaction gate splits every
event's attributes into **usage** (counts, durations, model names, token totals — may
flow to a central sink) and **content** (prompts, responses, user text — never leaves
the deployment). That boundary is the product's moat.

> **Status:** Phase 0 — core under construction. This module is developed in-tree inside
> the Kaimi repository and consumed by Kaimi as the first customer. It is designed to be
> extracted into its own repository unchanged (see *Splitting out* below).

## The zero-domain-import contract

The core is **domain-agnostic**. It knows about events, redaction, transport, storage,
and rendering — and nothing about proposals, opportunities, SAM.gov, NAICS, or any host
concept. Concretely:

- The core MUST NOT import any package under `github.com/Mawar2/Kaimi`.
- The dependency flows one way only: **the host imports the core, never the reverse.**
- Event names are opaque strings supplied by the host; the core never enumerates them.

This is enforced two ways: the separate `go.mod` makes a Kaimi import fail to resolve, and
a CI job (`telemetry-core`) fails the build if `go list -deps ./...` contains a Kaimi path.

## Splitting out (in-tree now → own repo later)

Because the boundary is enforced from the first commit, extraction is mechanical:

1. Create the new private repo (e.g. `github.com/Mawar2/kaimi-telemetry`).
2. Extract this directory **with history**: `git filter-repo --subdirectory-filter telemetry`.
3. The module path is already `github.com/Mawar2/kaimi-telemetry`, so no import churn.
4. Tag `v0.1.0` (SemVer).
5. In the host: drop the `replace` directive, `go get github.com/Mawar2/kaimi-telemetry@v0.1.0`,
   `go mod tidy`.
6. Port the `telemetry-core` CI guard into the new repo.

## Deploying the live stream behind Cloud Run

The SSE handler (added in T0.6) holds long-lived connections. On Cloud Run: raise the
request timeout, set CPU always-allocated (or min-instances ≥ 1), and rely on the
handler's heartbeat + `maxStream` graceful reconnect so the platform's request cap becomes
an invisible `EventSource` reconnect rather than a dropped stream.
