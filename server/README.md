# nas-gallery server

Single static Go binary providing the authenticated JSON API, incremental media scanner, on-demand JPEG thumbnail cache, and HTTP `Range` media streaming for `nas-gallery`.

## Quickstart

```bash
# Build (or download a release binary)
go build -o nas-gallery .

# 1. Initialize config.yaml, SQLite DB, and HMAC secret
./nas-gallery init

# 2. Edit config.yaml and set mediaPaths to your photo/video folders
#    e.g. mediaPaths: ["/volume1/photo/PS5"]

# 3. Create accounts (passwords are hidden at prompt and bcrypt-hashed)
./nas-gallery user add zach

# 4. Run the server
./nas-gallery serve -c config.yaml
```

## CLI Commands

| Command | Description |
|---|---|
| `nas-gallery init [-c config.yaml]` | Writes example `config.yaml` if absent, initializes SQLite DB, and generates the HMAC signing secret. |
| `nas-gallery user add <name> [-c path]` | Prompts for a password (hidden) and adds a user. |
| `nas-gallery user list [-c path]` | Lists configured usernames. |
| `nas-gallery user rm <name> [-c path]` | Removes a user and their tokens/favorites. |
| `nas-gallery serve [-c config.yaml]` | Starts the HTTP server and runs an initial background library scan. |
| `nas-gallery sampledata [-out dir]` | Generates a synthetic media folder (PNGs + minimal MP4/WebM fixtures) for local testing. |

## `config.yaml` Reference

| Key | Default | Description |
|---|---|---|
| `listenAddr` | `0.0.0.0:8080` | TCP bind address (`host:port`). |
| `serverName` | `My NAS` | Display name returned by `/api/me`. |
| `mediaPaths` | `[]` | List of root directories to index (mounted read-only by convention). |
| `dataDir` | `data` | Directory holding `nas-gallery.db` and the `thumbs/` JPEG cache. |
| `tokenTtlDays` | `30` | Bearer session lifetime when `rememberMe` is `false`. |
| `rememberTtlDays` | `180` | Bearer session lifetime when `rememberMe` is `true`. |
| `signedUrlTtlHours` | `12` | Lifetime of HMAC-signed `thumbUrl` and `streamUrl` links. |
| `rescanIntervalHours` | `6` | Periodic background rescan interval. |
| `ffmpegPath` | `""` | Optional `ffmpeg` binary path for video poster frames (empty = auto-detect in `PATH`; falls back to dark placeholder JPEG if unavailable). |
| `thumbGridEdge` | `480` | Long-edge size in pixels for `s=grid` JPEG thumbnails. |
| `thumbFullEdge` | `1280` | Long-edge size in pixels for `s=full` JPEG thumbnails. |

## Cross-Compilation (No cgo)

`modernc.org/sqlite` is pure Go, so static builds require no C toolchain:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o nas-gallery-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o nas-gallery-linux-arm64 .
```

## Security Notes

- Every `/api/*` JSON endpoint except `/api/health` and `/api/auth/login` requires a valid `Authorization: Bearer <token>` header.
- Bearer tokens are 256-bit random values stored as SHA-256 hashes in SQLite and can be revoked via `/api/auth/logout`.
- Passwords are stored as bcrypt hashes (cost 10); login is rate-limited per `(username, client IP)` starting at 5 failures (60s lockout, doubling up to 15 minutes).
- Media endpoints (`/api/media/{id}/stream` and `/thumb`) authenticate via short-lived HMAC-SHA256 signatures (`exp` + `sig`) and resolve item IDs strictly through the indexed database — never raw request paths.
