# Kaimi Desktop — build & run

**Last updated:** 2026-06-09

The desktop dashboard (`cmd/desktop`) is a **Wails v2 (Go)** app — see
[ADR-001](./adr-001-stack.md) for why. This ticket (#138) delivers the scaffold:
a window that reads a local Kaimi store and lists opportunities with their
derived pipeline stage. Design parity (#139), the offline editor, onboarding,
and cloud sync are later tickets.

## Layout

| Path | What it is |
|---|---|
| `internal/desktop/` | UI-agnostic backend (store read + list view-model). **No GUI dependency** — fully unit-tested, compiles everywhere (including Linux CI). |
| `cmd/desktop/main.go` | Wails entrypoint. Build-constrained to `windows || darwin`. |
| `cmd/desktop/main_unsupported.go` | Stub for other platforms so `go build/test ./...` stays green on the Linux CI runner without a Wails/CGO toolchain. |
| `cmd/desktop/frontend/` | **Vite + React** frontend implementing the handed-off design (`design-handoff/Kaimi-handoff`). Ported from the design prototypes into ES modules; the design's `tokens.css`/`ui.css`/`app.css`/`desktop.css` are copied **verbatim** under `src/styles/`. Figtree + IBM Plex Mono are bundled locally (`@fontsource`) → fully offline. |
| `cmd/desktop/frontend/src/` | `App.jsx` (shell: title bar, onboarding gate, offline bar, routing), `shared.jsx` (design-system components), `screens.jsx` (Sidebar, Opportunities + drawer), `proposals.jsx`, `workspace.jsx` (review gate), `onboarding.jsx` (6 steps), `editor.jsx` (draft editor), `data.js` (mock/fallback queue + proposals), `api.js` (maps the Go backend's opportunities into the design shape, falls back to the demo queue). |
| `cmd/desktop/wails.json` | Wails project config (Windows + `darwin/universal` targets; `frontend:install`/`build` run npm + Vite). |
| `cmd/desktop/frontend/dist/`, `node_modules/`, `build/` | **Generated** by `wails build` (Vite output, deps, icon/manifest/binary). Git-ignored; regenerated on demand. Branded app icon lands later. |

Data wiring: the **Opportunities** queue is read from the local store via the Go backend (`internal/desktop` → `internal/dashboard`); when the store is empty (or running in a plain browser), it falls back to the bundled demo queue so the UI is always populated. **Proposals** and **Workspace** run on mock data until Zone 2 agent events are wired (per INTENT.md).

## Prerequisites

- **Go 1.25+**
- **Node.js + npm** (the React frontend is built with Vite; `wails build`/`wails dev`
  run `npm install` and `npm run build` automatically via `wails.json`)
- **Wails v2 CLI:** `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`
  (the module is pinned to `v2.12.0` in `go.mod`)
- **Windows:** the **WebView2 runtime** (ships with Windows 11; evergreen on
  Windows 10). `wails doctor` confirms it.
- **macOS:** Xcode Command Line Tools.

Run `wails doctor` to verify your environment.

## Run (development)

From the project directory (hot reload):

```sh
cd cmd/desktop
wails dev                      # uses the default store path
wails dev -- -store C:\path\to\store   # point at a specific local store
```

You can also run the plain Go binary (no hot reload):

```sh
go run ./cmd/desktop -store C:\path\to\store
```

## Build (production)

```sh
cd cmd/desktop
wails build                              # current OS -> build/bin/kaimi-desktop[.exe]
wails build -platform darwin/universal   # macOS universal binary (run on macOS)
```

The output is a single binary in `cmd/desktop/build/bin/`.

## Choosing the store

The app reads an existing local Kaimi store directory (the same JSON layout the
pipeline writes — `<store>/queue/<id>.json`). Path resolution, highest priority
first:

1. `-store <path>` flag
2. `KAIMI_STORE_PATH` environment variable
3. Default: `<user-config-dir>/Kaimi/store` (e.g. `%AppData%\Kaimi\store` on Windows)

A missing or empty store is **not** an error — the app shows a calm empty state
and creates the directory. (Offline/empty states are slate, never amber; amber
is reserved for "a human is needed".)

## Tests, vet, lint

All run cross-platform because the testable logic lives in `internal/desktop`
(no Wails import):

```sh
go test ./...
go vet ./...
golangci-lint run
```

The Wails entrypoint compiles only on Windows/macOS; on Linux CI the stub keeps
`go build ./...` green.

## CI note (build job is optional this ticket)

`go test ./...` covers the backend on the standard Linux runner. A packaged
Windows/macOS `wails build` job (with the WebView2/Xcode toolchains) is a
later addition; it is not required for #138.
