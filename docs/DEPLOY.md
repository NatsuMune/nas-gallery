# Deployment & Usage Guide

`nas-gallery` has two halves:

1. **PWA Frontend (`web/`)** — a static installable web app hosted on **GitHub Pages** (free).
2. **Go Backend (`server/`)** — a single static binary running directly on your **NAS** (no Docker required), exposed over an **outbound-only HTTPS tunnel**.

Your photos and videos never leave your NAS except when streamed to a signed-in user.

---

## Step 1 — Host the PWA on GitHub Pages

1. Push this repository to GitHub (public repo for free GitHub Pages, or private on GitHub Pro).
   Your media and passwords are never in the repository.
2. In your GitHub repo go to **Settings → Pages → Build and deployment → Source** and select **GitHub Actions**.
3. Push to `main` (or run **Actions → Deploy PWA to GitHub Pages → Run workflow**).
4. Your PWA is now live at `https://<your-github-username>.github.io/<repo-name>/`.

---

## Step 2 — Install & Configure the Backend on Synology DSM (No Docker)

### Which binary?

- Most Intel/AMD Synology models (DS220+, DS423+, DS920+, DS923+, DS1522+, etc.): **`nas-gallery-linux-amd64`**
- ARM64 Synology models (DS223, DS423, DS218, etc.): **`nas-gallery-linux-arm64`**

Check via SSH with `uname -m` (`x86_64` → `amd64`, `aarch64` → `arm64`).

### Setup over SSH

1. Download the binary from GitHub Releases (or cross-compile locally from `server/`):

   ```bash
   # Cross-compile from your laptop if you haven't tagged a release yet:
   cd server
   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o nas-gallery .
   ```

2. Copy it to your NAS (e.g. `/volume1/homes/<you>/nas-gallery/`):

   ```bash
   mkdir -p /volume1/homes/<you>/nas-gallery
   cd /volume1/homes/<you>/nas-gallery
   chmod +x nas-gallery
   ```

3. Initialize config, SQLite DB, and HMAC secret:

   ```bash
   ./nas-gallery init
   ```

4. Edit `config.yaml` and point `mediaPaths` at your photo/video folders (for example PS5 captures):

   ```yaml
   listenAddr: 127.0.0.1:8080 # bind localhost if using Tailscale/Cloudflare on the NAS
   serverName: My NAS
   mediaPaths:
     - /volume1/photo/PS5
   dataDir: /volume1/homes/<you>/nas-gallery/data
   ```

   > Note: `nas-gallery` only **reads** `mediaPaths`. Thumbnails and the SQLite DB live in `dataDir`.

5. Create accounts (one for you, one for your wife — favorites are per-user):

   ```bash
   ./nas-gallery user add zach
   ./nas-gallery user add wife
   ```

6. Test running it manually:

   ```bash
   ./nas-gallery serve -c /volume1/homes/<you>/nas-gallery/config.yaml
   ```

   Verify from another terminal or browser: `curl http://<nas-lan-ip>:8080/api/health` (if bound to `0.0.0.0:8080`) or `curl http://127.0.0.1:8080/api/health`.

### Auto-start on Synology DSM Boot (No Docker)

1. Open **DSM → Control Panel → Task Scheduler**.
2. **Create → Triggered Task → User-defined script**.
3. General: Name `nas-gallery`, User: your user (or `root` if reading `/volume1/photo` requires it), Event: **Boot-up**, Enabled: checked.
4. Task Settings → Run command:

   ```bash
   /volume1/homes/<you>/nas-gallery/nas-gallery serve -c /volume1/homes/<you>/nas-gallery/config.yaml >> /volume1/homes/<you>/nas-gallery/server.log 2>&1
   ```

5. Save, select the task, and click **Run** to start it immediately.

---

## Step 3 — Expose the API Safely via an Outbound Tunnel

GitHub Pages runs over HTTPS, so browsers require your NAS API URL to be HTTPS too (no mixed content), without opening router ports.

### Option A: Tailscale Funnel (Recommended — Free, No Domain Needed)

1. Install **Tailscale** from Synology **Package Center** and sign in (`sudo tailscale up` via SSH if needed).
2. Enable **HTTPS** and **Funnel** in your Tailscale admin DNS/ACL settings.
3. On the NAS over SSH, proxy public HTTPS traffic to your local `nas-gallery` port:

   ```bash
   sudo tailscale funnel --bg 8080
   ```

4. Tailscale gives you a permanent HTTPS URL like `https://<your-nas>.<tailnet>.ts.net`.
5. Keep `listenAddr: 127.0.0.1:8080` in `config.yaml` — only Tailscale reaches it locally.

### Option B: Cloudflare Tunnel (Custom Domain like `photos.yourdomain.com`)

1. Add a domain you own to a free Cloudflare account.
2. In **Cloudflare Zero Trust → Networks → Tunnels**, create a tunnel and point `api-photos.yourdomain.com` → `http://127.0.0.1:8080`.
3. Run `cloudflared` on the NAS (binary or DSM task) with the provided tunnel token.

---

## Step 4 — Daily Usage & Installing the PWA

1. Open your GitHub Pages URL (`https://<user>.github.io/<repo>/`) on your phone or desktop.
2. Enter:
   - **Server URL:** `https://<your-nas>.<tailnet>.ts.net` (from Step 3)
   - **Username & Password:** created in Step 2
   - Check **Remember me for 180 days** if on a personal device.
3. **Install as a PWA:**
   - **Desktop Chrome / Edge:** click the **Install** button in the header (or the address bar install icon).
   - **iOS Safari:** tap **Share → Add to Home Screen**.
   - **Android Chrome:** tap **Install** in the header or browser menu.
4. **Controls & Shortcuts:**
   - **Browse:** folders in the sidebar (desktop) or folder cards + breadcrumbs (mobile); sort by Date/Name/Size/Type and filter All/Photos/Videos.
   - **Favorites:** click the heart on any tile or in the lightbox; view yours under **Favorites**.
   - **Video Player:** click/Space/`K` play-pause, `J`/`L` or `−30s`/`+30s` buttons jump 30 seconds, `←`/`→` jump 10 seconds, `↑`/`↓` volume, `M` mute, `F` or double-click fullscreen, playback speed `0.5×–2×`, automatic position resume across sessions, and **Download** / **Download to watch** fallback for unsupported codecs.
   - **Search:** press `/` or use the header search bar.
