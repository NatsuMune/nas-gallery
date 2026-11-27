# AGENTS.md — conventions for anyone (human or agent) working in this repo

## Project

`nas-gallery` (working title) is a generic, open-source, self-hosted photo/video gallery:

- `web/` — PWA frontend (React + Vite + TypeScript), deployed to GitHub Pages.
- `server/` — Go backend, a single static binary run on a NAS (Synology DSM without Docker),
  exposing an authenticated JSON API + media streaming.

## Source of truth

- `docs/API_CONTRACT.md` defines every endpoint, field, auth rule, and behavior.
  Frontend and backend are built in parallel from this contract — treat it as law.
- If the contract is ambiguous: make the most sensible call, record the decision in your
  commit message and final report. Do not stall.

## Ownership / lanes

- Backend work: only `server/**`.
- Frontend work: only `web/**`.
- Shared docs (`docs/`, root files): owned by the coordinator. Propose changes in your
  report; do not edit directly.

## Build & test commands

Backend (from `server/`):

```
go build ./...
go vet ./...
go test ./...
```

Frontend (from `web/`):

```
npm ci
npm run build
npm run test          # vitest unit tests
npx playwright install chromium   # once
npx playwright test   # e2e against the mock server
```

A non-zero exit from any of these means the task is not done.

## Commits

- Conventional Commits (`feat:`, `fix:`, `test:`, `docs:`, `chore:`, `refactor:`).
- Small, clean commits. Leave the worktree clean (everything committed) when you finish.
- Never commit: media files, SQLite DBs, binaries, `node_modules`, secrets, `.env`,
  thumb caches, generated `dist/` output.

## Quality bar

- Go: idiomatic, `gofmt`-clean, stdlib-first, errors wrapped with context.
- TypeScript: `strict: true`, no `any` unless unavoidable, no unused deps.
- CSS: hand-rolled with design tokens; no UI frameworks.
- Tests must be meaningful (behavior, not line coverage).

## Security invariants (never violate)

- Every `/api` JSON endpoint except `/api/health` requires a valid Bearer token.
- Media bytes are only served via valid, unexpired HMAC-signed URLs.
- Tokens are stored hashed (SHA-256); passwords bcrypt.
- No path traversal: item ids resolve to indexed files only, never raw request paths.
