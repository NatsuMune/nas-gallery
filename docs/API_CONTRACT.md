# API Contract — nas-gallery v1

This is the single source of truth for the HTTP API. The backend implements it; the
frontend (and its mock server) consumes it exactly as written.

## Conventions

- All endpoints are prefixed with `/api`.
- All request/response bodies are JSON unless noted (media endpoints stream bytes).
- Dates: RFC 3339 UTC strings. IDs: opaque strings (`"root"`, `"f_12"`, `"m_99"`).
- Field names: camelCase. Unknown fields must be tolerated by both sides.
- Trailing slashes never matter; unknown routes → 404.

### CORS

All `/api` endpoints (including media) send:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type
Access-Control-Expose-Headers: Content-Disposition, Content-Length, Accept-Ranges
```

`OPTIONS` preflight → `204 No Content`. No cookies are ever used (the frontend is
cross-origin on GitHub Pages; third-party cookies are unreliable). Auth is Bearer
tokens for JSON endpoints and signed URLs for media endpoints.

### Errors

Any error → JSON `{"error":"<human readable>"}` with an appropriate status:
`400` bad request, `401` invalid/missing/expired credentials or signature,
`404` not found, `429` rate-limited (+ `Retry-After` seconds), `500` internal.

### Pagination

List endpoints accept `page` (1-based) and `pageSize` (default 100, max 500) and return:

```json
{"items":[...], "page":1, "pageSize":100, "total":1234}
```

### Sorting (item lists)

`sort=name|date|size|type`, `order=asc|desc`. Defaults: `date`, `desc` (newest first).
`date` = file modification time. `type` sorts photos before videos (then by name).
`type=all|photo|video` filters item lists (default `all`).

---

## Public

### `GET /api/health`

No auth. Always 200:

```json
{"ok": true, "version": "1.0.0"}
```

---

## Auth

### `POST /api/auth/login`

```json
{"username":"zach", "password":"…", "rememberMe": true}
```

→ `200`:

```json
{"token":"<64 hex chars>", "expiresAt":"2026-12-27T00:00:00Z"}
```

- `rememberMe: true` → long TTL (180 days); otherwise 30 days (server config).
- Bad credentials → `401 {"error":"Invalid username or password"}` (identical for
  unknown user / wrong password).
- Rate limiting: 5 failed attempts per (username + client IP) → lockout starting at
  60 s, doubling per subsequent failure, capped at 15 min. Locked → `429` + `Retry-After`.

### `POST /api/auth/logout`

Auth required. Revokes the presented token. → `204 No Content`.

### `GET /api/me`

Auth required.

```json
{"user":{"id":1,"username":"zach"}, "server":{"name":"My NAS","version":"1.0.0"}}
```

---

## Folders

The library is a virtual folder tree. Each configured media root is a top-level child
of the virtual root folder (id `"root"`). `GET /api/folders/{id}` with `id=root`
returns the media roots.

### `GET /api/folders/{id}`

Auth required. → `200`:

```json
{
  "folder": {"id":"f_3","name":"Screenshots","parentId":"root","relativePath":"Screenshots","fileCount":342,"childCount":7,"coverItemId":"m_88"},
  "children": [ {"id":"f_4","name":"Astro's Playroom","parentId":"f_3","relativePath":"Screenshots/Astro's Playroom","fileCount":120,"childCount":0,"coverItemId":"m_12"} ],
  "breadcrumbs": [ {"id":"root","name":"Home","parentId":null,"relativePath":"","fileCount":0,"childCount":2,"coverItemId":null},
                   {"id":"f_3","name":"Screenshots","parentId":"root","relativePath":"Screenshots","fileCount":342,"childCount":7,"coverItemId":"m_88"} ]
}
```

- `children` = direct subfolders only (never files). Empty array if none.
- `coverItemId` = id of an image suitable as the folder tile art (a photo if available,
  else the poster frame of a video, else `null`).
- `breadcrumbs` includes the requested folder itself. For `id=root`: `folder` is `null`,
  `breadcrumbs` is `[]`.
- `fileCount` counts media files directly inside the folder (not recursive).

---

## Items

Item object (all fields always present; nullable ones may be `null`):

```json
{
  "id":"m_99",
  "name":"VIDEO_20260218_203014.mp4",
  "type":"video",
  "size":48392877,
  "modTime":"2026-02-18T20:30:14Z",
  "width":1920,
  "height":1080,
  "durationMs":30400,
  "favorite":true,
  "folderId":"f_4",
  "thumbUrl":"/api/media/m_99/thumb?s=grid&exp=1769472000&sig=<hex>",
  "streamUrl":"/api/media/m_99/stream?exp=1769472000&sig=<hex>"
}
```

- `type`: `"photo"` or `"video"`.
- `width`/`height`/`durationMs`: best-effort metadata, `null` when undetectable.
- `favorite`: the *current user's* favorite state (favorites are per-user).
- `thumbUrl`/`streamUrl`: freshly signed at response time; paths (not absolute URLs) —
  the frontend prefixes its configured server origin. TTL: 12h default.

### `GET /api/folders/{id}/items`

Auth required. Supports sort/order/type/page/pageSize. → `200`:

```json
{"items":[Item, …], "page":1, "pageSize":100, "total":342, "folder":Folder}
```

`folder` is the folder object (same as in `GET /api/folders/{id}`); `null` for root
(uses `folderId=root`; the virtual root lists files sitting directly in a media root
only when a media root points at a flat directory).

### `GET /api/items/{id}`

Auth required. → `200`: single Item (fresh signed URLs).

### `GET /api/favorites`

Auth required. All items the current user favorited, newest first by default.
Supports sort/order/page/pageSize. Same response shape as folder items (no `folder`).

### `GET /api/search?q=…`

Auth required. Case-insensitive substring match on filename. Same response shape
(min 1 char, otherwise `400`).

### `PUT /api/items/{id}/favorite`

Auth required. Body `{"favorite": true}`. Sets/clears the current user's favorite.
→ `200 {"favorite":true}`.

---

## Media (signed URLs — no Bearer header)

`<img src>` / `<video src>` cannot send Authorization headers, so media endpoints
authenticate via the `exp` + `sig` query params that the API embeds in `thumbUrl` /
`streamUrl`. The signature is HMAC-SHA256 over
`"<itemId>|<variant>|<extra-params-sorted>|<exp>"` with a server-side secret.
Invalid or expired → `401` JSON error. These endpoints must fully support CORS
(see above) plus `Range` requests.

The frontend should refetch item data (fresh URLs) on `401` from a media URL.

### `GET /api/media/{id}/stream?exp=&sig=[&download=1]`

Original file bytes.

- Correct `Content-Type` by extension.
- `Accept-Ranges: bytes`; honors `Range` → `206 Partial Content` with correct
  `Content-Range` (this is what makes video seeking work).
- `download=1` → `Content-Disposition: attachment; filename*=UTF-8''<urlencoded name>`.
- `Cache-Control: private, max-age=3600`.

### `GET /api/media/{id}/thumb?s=grid|full&exp=&sig=`

JPEG thumbnail: `grid` = long edge 480px, `full` = long edge 1280px (q≈80).

- Photos: generated from the original, cached on disk, served with
  `Cache-Control: private, max-age=86400`.
- Videos: poster frame extracted via ffmpeg if available; otherwise a generated dark
  placeholder JPEG (always `200`, never `404`; the frontend overlays a video icon).
- Unsupported-to-decode photos (e.g. HEIC on a non-decoding backend): same placeholder
  behavior. The original stream endpoint always serves the raw file.

---

## Library

### `POST /api/library/scan`

Auth required. Triggers an incremental rescan (idempotent; returns immediately).
→ `200 {"scanning":true}`.

### `GET /api/library/status`

Auth required. → `200`:

```json
{"scanning":false, "lastScanAt":"2026-03-01T00:00:00Z", "counts":{"photo":1204,"video":318}}
```

---

## Server behavior (not HTTP)

### CLI

```
nas-gallery init                          # write example config if absent, create DB, generate secret
nas-gallery user add <name>               # prompts for password (hidden), bcrypt, stores in DB
nas-gallery user list
nas-gallery user rm <name>
nas-gallery serve [-c config.yaml]
```

### config.yaml

```yaml
listenAddr: 0.0.0.0:8080
serverName: My NAS
mediaPaths:
  - /volume1/photo
dataDir: /volume1/homes/admin/nas-gallery   # DB, thumb cache, secret
tokenTtlDays: 30
rememberTtlDays: 180
signedUrlTtlHours: 12
rescanIntervalHours: 6
ffmpegPath: ""                              # empty = auto-detect in PATH, optional
thumbGridEdge: 480
thumbFullEdge: 1280
```

### Scanner

- Incremental walk of all `mediaPaths`; indexes by (path, size, mtime); removes
  deleted files; runs at startup (async), every `rescanIntervalHours`, and on
  `POST /api/library/scan`.
- Excludes: `@eaDir`, `#recycle`, `#snapshot`, `System Volume Information`,
  `lost+found`, dotfiles/dot-dirs, `.DS_Store`, `Thumbs.db`.
- Recognized extensions:
  - photo: `jpg jpeg png gif webp bmp tiff avif heic heif`
  - video: `mp4 webm mov mkv m4v avi`

### Metadata

- Photo dimensions: parse image headers (`image.DecodeConfig` + x/image); `null` on
  failure or unsupported format.
- Video duration: native parsing of MP4 (`moov/mvhd`) and WebM/Matroska (EBML
  Segment → Info → Duration); `null` on failure. No ffprobe required.
- Video width/height: best effort from MP4 `tkhd`; `null` otherwise.

### Playback compatibility note (frontend behavior)

MP4 (H.264/AAC) and WebM (VP8/VP9) direct-play in all modern browsers. MOV/MKV/AVI and
some MP4 variants (HEVC) may not — the frontend must catch playback errors and show a
fallback UI with a **download** action. Transcoding is explicitly out of scope for v1.
