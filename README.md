# nas-gallery

An open-source, self-hosted photo and video gallery for your NAS — a pure web app (PWA) hosted on **GitHub Pages**, backed by a **single static Go binary** running directly on your NAS (no Docker required).

Your media stays on your NAS and is only accessible after login.

## Features

- **Pure Web App (PWA):** installable on desktop, iOS, and Android; caches the app shell and thumbnails for fast browsing; deploys to GitHub Pages via GitHub Actions.
- **Zero-Docker NAS Backend:** single static Go binary (`CGO_ENABLED=0`, pure-Go SQLite via `modernc.org/sqlite`) that cross-compiles cleanly for Synology `amd64`/`arm64`.
- **Authentication & Security:**
  - Per-user accounts (`nas-gallery user add`) with bcrypt passwords and revocable Bearer tokens (stored as SHA-256 hashes).
  - Login rate limiting (`429` + `Retry-After` lockout after 5 failed attempts).
  - Media bytes served only via short-lived HMAC-SHA256 signed URLs (`exp` + `sig`).
- **Media Browsing & Custom Video Player:**
  - Virtual folder tree + breadcrumbs, sorting (`date`, `name`, `size`, `type`), type filtering (`all`, `photo`, `video`), filename search, and per-user favorites.
  - Proper HTTP `Range` (`206 Partial Content`) streaming for instant seeking in MP4/WebM recordings.
  - Custom video player with **±30s jump buttons**, draggable seek bar with buffered ranges, playback speed (`0.5×–2×`), fullscreen, keyboard shortcuts (`Space`/`K`, `J`/`L`, `←`/`→`, `M`, `F`), resume playback position, and a **Download to watch** fallback for unsupported codecs.
  - Incremental media scanner (skips `@eaDir`, `#recycle`, `#snapshot`, dotfiles) with native MP4/WebM duration parsing and on-demand JPEG thumbnail caching (optional `ffmpeg` for video poster frames).

## Repository Layout

| Path | Description |
|---|---|
| [`server/`](server/) | Static Go backend (API, auth, scanner, JPEG thumbs, HTTP Range streaming, CLI) |
| [`web/`](web/) | React + TypeScript + Vite PWA frontend, mock API server, unit + Playwright e2e tests |
| [`docs/API_CONTRACT.md`](docs/API_CONTRACT.md) | Full HTTP API & server behavior specification |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | System shape, auth model, and SQLite schema |
| [`docs/DEPLOY.md`](docs/DEPLOY.md) | Step-by-step GitHub Pages + Synology DSM + Tailscale/Cloudflare deployment guide |

## Quick Local Demo

Terminal 1 — start the backend with synthetic sample media:

```bash
cd server
go run . sampledata -out ./sample-media
go run . init
# Edit config.yaml and set mediaPaths: ["./sample-media"]
go run . user add demo
go run . serve
```

Terminal 2 — start the frontend dev server:

```bash
cd web
npm ci
npm run dev
```

Open `http://localhost:5173`, enter Server URL `http://127.0.0.1:8080`, and sign in with `demo` and the password you set.

## Testing

```bash
# Backend
cd server && go build ./... && go vet ./... && go test ./...

# Frontend
cd web && npm ci && npm run build && npm run test && npx playwright test
```

## License

[MIT](LICENSE)
