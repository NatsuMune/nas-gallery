# Architecture

## System shape

```
┌─────────────────────────┐         ┌──────────────────────────┐
│ Browser / installed PWA │         │   NAS (Synology DSM)     │
│  GitHub Pages           │         │                          │
│  https://<user>.        │  HTTPS  │  nas-gallery binary      │
│  github.io/<repo>       │◄───────►│  (no Docker)             │
│                         │         │   │        │             │
│  React + Vite + TS      │         │   │        │             │
│  service worker,        │         │  SQLite   mediaPaths     │
│  manifest               │         │  thumbs   (/volume1/...) │
└─────────────────────────┘         │   │                      │
         ▲                          └───┼──────────────────────┘
         │                              │ outbound-only tunnel
         │                     ┌────────▼─────────┐
         └─────────────────────┤ Tailscale Funnel │ (or Cloudflare Tunnel)
                               │  public HTTPS    │
                               │  nas.xxx.ts.net  │
                               └──────────────────┘
```

- The frontend is a **pure web app (PWA)** — an installable website, not a native app.
  Hosted statically on GitHub Pages; deployable to any static host.
- The backend is a **single static Go binary** (cross-compiled, zero runtime deps,
  no cgo) that runs directly on the NAS. On Synology: DSM Task Scheduler "boot-up"
  task or systemd unit — Docker not required.
- Public reachability via an **outbound-only tunnel** (Tailscale Funnel by default:
  free, stable URL, no domain needed; Cloudflare Tunnel documented as an alternative
  for a custom domain). No inbound ports are ever opened on the NAS or router.

## Auth model (why no cookies)

The PWA origin (`github.io`) differs from the API origin (the tunnel URL), and modern
browsers block third-party cookies, so session cookies are unreliable. Therefore:

- **JSON API:** `Authorization: Bearer <token>` — opaque random token, stored hashed
  (SHA-256) server-side in SQLite, revocable, TTL 30d / 180d ("remember me").
- **Media bytes:** HMAC-SHA256-signed, expiring URLs (`exp` + `sig` query params) so
  `<img>` / `<video>` elements work without headers. Signed fresh with every API
  response (12h TTL), so a browsing session never breaks.
- Login is rate-limited; passwords are bcrypt. The NAS never sees inbound traffic.

## Repo layout

```
server/   Go backend (owns: API, auth, scanner, thumbs, streaming, CLI)
web/      PWA frontend (owns: UI, API client, mock server, e2e tests)
docs/     Shared contracts and architecture (coordinator-owned)
.github/  CI: Pages deploy, release cross-compilation (added at integration)
```

## Data model (SQLite)

- `users(id, username, password_hash, created_at)`
- `tokens(id, user_id, token_hash, expires_at)` — revocable sessions
- `folders(id, path, name, parent_id)` — mirrors the directory tree of all media roots
- `items(id, folder_id, path, name, ext, type, size, mtime, width, height, duration_ms)`
  — indexed by (path, size, mtime) for incremental scans
- `favorites(user_id, item_id)` — per-user
- `meta(key, value)` — HMAC secret, schema version, last scan time

Thumbnail cache lives on disk under `dataDir/thumbs/<id>_<variant>.jpg`.

## v1 scope decisions

- **Direct play** MP4 (H.264) and WebM — what PS5 captures produce — with proper
  HTTP range streaming (this is what fixes seeking/jumping in players).
  Unsupported codecs get a graceful "download to watch" fallback UI. No transcoding.
- Favorites are **per-user** (two accounts: owner + wife).
- Search is filename-based; EXIF/location/face search is future work.
- The backend never transcodes or re-encodes anything in v1; ffmpeg is optional and
  used only for video poster frames when present.

## Deployment model (final docs ship at integration)

1. Frontend → GitHub Pages via GitHub Actions on push to `main`.
2. Backend → GitHub Releases with cross-compiled binaries
   (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64).
3. NAS setup: download binary → `nas-gallery init` → `nas-gallery user add` →
   edit `config.yaml` (`mediaPaths`) → Task Scheduler boot task → tunnel.
