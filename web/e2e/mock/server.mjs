// Mock nas-gallery API server for frontend dev + e2e tests.
//
// Plain Node (no deps). Implements the full docs/API_CONTRACT.md surface
// in memory:
//   - auth (Bearer tokens, login rate limiting, demo/demo)
//   - folders tree, items with sort/order/type/page/pageSize
//   - favorites, search, library scan/status
//   - HMAC-signed media URLs with Range streaming
//
// Mock simplifications (documented on purpose):
//   - Thumbnails are tiny PNGs from an inline encoder (the contract lets the
//     frontend treat thumbUrl/streamUrl as opaque URLs; content type of the
//     bytes is irrelevant to the client). The real backend serves JPEG.
//   - Photos carry plausible-but-fake pixel dimensions, like a real library
//     would; the bytes themselves are small gradients.
//   - "broken.avi" is signed with a wrong secret so its stream always 401s —
//     it exists to exercise the player's error-fallback UI.
//
// Credentials: demo / demo
import http from 'node:http';
import crypto from 'node:crypto';
import { deflateSync } from 'node:zlib';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const PORT = process.env.PORT ? Number(process.env.PORT) : 8787;
const SECRET = 'mock-hmac-secret';
const WRONG_SECRET = 'mock-hmac-secret-wrong'; // used for the broken item
const SIGNED_TTL_SEC = 12 * 60 * 60; // 12h, per contract
const VERSION = '1.0.0';
const VERBOSE = process.env.MOCK_VERBOSE === '1';

const __dirname = dirname(fileURLToPath(import.meta.url));
const FIXTURES = join(__dirname, '..', 'fixtures');
const sunsetMp4 = readFileSync(join(FIXTURES, 'sunset.mp4'));
const wavesWebm = readFileSync(join(FIXTURES, 'waves.webm'));

// ---------------------------------------------------------------------------
// Minimal PNG encoder (truecolor RGB, no interlace)
// ---------------------------------------------------------------------------
const CRC_TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

function crc32(buf) {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function pngChunk(type, data) {
  const out = Buffer.alloc(8 + data.length + 4);
  out.writeUInt32BE(data.length, 0);
  out.write(type, 4, 'ascii');
  data.copy(out, 8);
  out.writeUInt32BE(crc32(out.subarray(4, 8 + data.length)), 8 + data.length);
  return out;
}

function makePng(width, height, pixel) {
  const raw = Buffer.alloc(height * (1 + width * 3));
  let o = 0;
  for (let y = 0; y < height; y++) {
    raw[o++] = 0; // filter: none
    for (let x = 0; x < width; x++) {
      const [r, g, b] = pixel(x, y);
      raw[o++] = r;
      raw[o++] = g;
      raw[o++] = b;
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 2; // color type: truecolor
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk('IHDR', ihdr),
    pngChunk('IDAT', deflateSync(raw, { level: 9 })),
    pngChunk('IEND', Buffer.alloc(0)),
  ]);
}

// Pleasant dark gradient art for photo tiles.
function photoPng(seed) {
  const w = 128;
  const h = 128;
  const hueA = (seed * 47) % 360;
  const hueB = (hueA + 60 + (seed % 40)) % 360;
  const hsl = (h_, s, l) => {
    const a = (s * Math.min(l, 1 - l)) / 100;
    const f = (n) => {
      const k = (n + h_ / 30) % 12;
      return l - a * Math.max(-1, Math.min(k - 3, Math.min(9 - k, 1)));
    };
    return [Math.round(f(0) * 255), Math.round(f(8) * 255), Math.round(f(4) * 255)];
  };
  const cA = hsl(hueA, 55, 42);
  const cB = hsl(hueB, 60, 22);
  return makePng(w, h, (x, y) => {
    const t = (x / w + y / h) / 2;
    const diag = Math.hypot(x - w / 2, y - h / 2) / (Math.SQRT2 * (w / 2));
    const glow = Math.max(0, 1 - Math.abs(diag - 0.55) * 3) * 0.5;
    return [
      Math.round(cA[0] * (1 - t) + cB[0] * t + glow * 90),
      Math.round(cA[1] * (1 - t) + cB[1] * t + glow * 100),
      Math.round(cA[2] * (1 - t) + cB[2] * t + glow * 140),
    ];
  });
}

function videoPosterPng(seed) {
  const w = 128;
  const h = 128;
  const base = [13 + (seed % 3) * 8, 16 + (seed % 5) * 6, 30 + (seed % 4) * 12];
  return makePng(w, h, (x, y) => {
    const t = y / h;
    const glow = Math.max(0, 1 - Math.hypot(x - w / 2, y - h / 2) / 70);
    return [
      Math.round(base[0] * (1 - t * 0.7) + glow * 40),
      Math.round(base[1] * (1 - t * 0.7) + glow * 60),
      Math.round(base[2] * (1 - t * 0.5) + glow * 110),
    ];
  });
}

// ---------------------------------------------------------------------------
// In-memory library
// ---------------------------------------------------------------------------
const users = [{ id: 1, username: 'demo', password: 'demo' }];
const tokens = new Map(); // token -> { userId, expiresAt }
const favoritesByUser = new Map([[1, new Set()]]); // userId -> Set<itemId>
const loginFailures = new Map(); // "user|ip" -> { count, lockedUntil }

/** @type {Array<{id:string,name:string,parentId:string|null,relativePath:string}>} */
const folderDefs = [
  { id: 'f_photos', name: 'Photos', parentId: 'root', relativePath: 'Photos' },
  { id: 'f_2024', name: '2024', parentId: 'f_photos', relativePath: 'Photos/2024' },
  { id: 'f_2025', name: '2025', parentId: 'f_photos', relativePath: 'Photos/2025' },
  { id: 'f_empty', name: 'Empty Album', parentId: 'f_photos', relativePath: 'Photos/Empty Album' },
  { id: 'f_videos', name: 'Videos', parentId: 'root', relativePath: 'Videos' },
];

/** @type {Array<any>} raw item records (before URL signing) */
const items = [];
let itemSeq = 0;

function addItem(rec) {
  itemSeq += 1;
  const id = `m_${itemSeq}`;
  items.push({ id, favorite: false, ...rec });
  return items[items.length - 1];
}

// Photos in Photos/2025 — 130 of them so infinite scroll (pageSize 100) kicks in.
{
  const png = photoPng(7);
  for (let i = 0; i < 130; i++) {
    const day = 1 + (i % 27);
    const month = 1 + (i % 12);
    const seq = String(i + 1).padStart(4, '0');
    const date = `2025${String(month).padStart(2, '0')}${String(day).padStart(2, '0')}`;
    addItem({
      name: `IMG_${date}_${seq}.jpg`,
      type: 'photo',
      folderId: 'f_2025',
      size: png.length + 2_400_000 + (i * 997) % 4_000_000, // plausible camera sizes
      modTime: `2025-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}T${String(9 + (i % 12)).padStart(2, '0')}:${String(i % 60).padStart(2, '0')}:00Z`,
      width: 4032,
      height: i % 3 === 0 ? 3024 : 2268,
      durationMs: null,
      bytes: png,
      mime: 'image/png',
    });
  }
}

// Photos in Photos/2024 — 60.
{
  const png = photoPng(23);
  for (let i = 0; i < 60; i++) {
    const month = 1 + (i % 12);
    const day = 1 + ((i * 3) % 27);
    const seq = String(i + 1).padStart(4, '0');
    addItem({
      name: `IMG_2024${String(month).padStart(2, '0')}${String(day).padStart(2, '0')}_${seq}.jpg`,
      type: 'photo',
      folderId: 'f_2024',
      size: png.length + 1_800_000 + (i * 733) % 3_000_000,
      modTime: `2024-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}T14:${String(i % 60).padStart(2, '0')}:00Z`,
      width: 4000,
      height: 3000,
      durationMs: null,
      bytes: png,
      mime: 'image/png',
    });
  }
}

// A few loose photos directly in Photos/.
{
  const png = photoPng(41);
  const names = ['beach-sunset.jpg', 'city-lights.jpg', 'forest-path.jpg', 'mountain-top.jpg', 'night-sky.jpg'];
  for (let i = 0; i < names.length; i++) {
    addItem({
      name: names[i],
      type: 'photo',
      folderId: 'f_photos',
      size: png.length + 2_000_000 + i * 100_000,
      modTime: `2026-0${1 + (i % 3)}-1${i % 9}T10:00:00Z`,
      width: 3840,
      height: 2160,
      durationMs: null,
      bytes: png,
      mime: 'image/png',
    });
  }
}

// Videos.
addItem({
  name: 'sunset.mp4',
  type: 'video',
  folderId: 'f_videos',
  size: sunsetMp4.length,
  modTime: '2026-02-18T20:30:14Z',
  width: 320,
  height: 180,
  durationMs: 40000,
  bytes: sunsetMp4,
  mime: 'video/mp4',
  poster: videoPosterPng(1),
});
addItem({
  name: 'waves.webm',
  type: 'video',
  folderId: 'f_videos',
  size: wavesWebm.length,
  modTime: '2026-01-05T09:12:00Z',
  width: 320,
  height: 180,
  durationMs: 20000,
  bytes: wavesWebm,
  mime: 'video/webm',
  poster: videoPosterPng(2),
});
addItem({
  name: 'broken.avi',
  type: 'video',
  folderId: 'f_videos',
  size: 4_000_000,
  modTime: '2026-03-02T11:00:00Z',
  width: 1280,
  height: 720,
  durationMs: 65000,
  bytes: Buffer.alloc(0), // never actually served
  mime: 'video/x-msvideo',
  poster: videoPosterPng(3),
  broken: true, // signed with the wrong secret -> stream always 401s
});

// Two preset favorites so the Favorites view is not empty on first visit.
items[3].favorite = true;
items[40].favorite = true;
favoritesByUser.get(1).add(items[3].id);
favoritesByUser.get(1).add(items[40].id);

const folderById = new Map();
const ROOT = { id: 'root', name: 'Home', parentId: null, relativePath: '' };
for (const f of folderDefs) folderById.set(f.id, f);

const itemsByFolder = new Map();
for (const it of items) {
  if (!itemsByFolder.has(it.folderId)) itemsByFolder.set(it.folderId, []);
  itemsByFolder.get(it.folderId).push(it);
}

function folderStats(id) {
  const list = itemsByFolder.get(id) ?? [];
  const children = folderDefs.filter((f) => f.parentId === id);
  const sorted = [...list].sort((a, b) => (a.modTime < b.modTime ? 1 : -1));
  const cover = sorted.find((i) => i.type === 'photo') ?? sorted[0] ?? null;
  return { fileCount: list.length, childCount: children.length, coverItemId: cover ? cover.id : null };
}

function serializeFolder(f) {
  if (!f) return null;
  const stats = folderStats(f.id);
  return { id: f.id, name: f.name, parentId: f.parentId, relativePath: f.relativePath, ...stats };
}

// ---------------------------------------------------------------------------
// Signed URLs
// ---------------------------------------------------------------------------
function sign(item, variant, extra) {
  const exp = Math.floor(Date.now() / 1000) + SIGNED_TTL_SEC;
  const secret = item.broken ? WRONG_SECRET : SECRET;
  const sig = crypto
    .createHmac('sha256', secret)
    .update(`${item.id}|${variant}|${extra}|${exp}`)
    .digest('hex');
  return { exp, sig };
}

function serializeItem(item) {
  const thumb = sign(item, 'thumb', 's=grid');
  const stream = sign(item, 'stream', '');
  return {
    id: item.id,
    name: item.name,
    type: item.type,
    size: item.size,
    modTime: item.modTime,
    width: item.width,
    height: item.height,
    durationMs: item.durationMs,
    favorite: favoritesByUser.get(1).has(item.id),
    folderId: item.folderId,
    thumbUrl: `/api/media/${item.id}/thumb?s=grid&exp=${thumb.exp}&sig=${thumb.sig}`,
    streamUrl: `/api/media/${item.id}/stream?exp=${stream.exp}&sig=${stream.sig}`,
  };
}

// ---------------------------------------------------------------------------
// HTTP plumbing
// ---------------------------------------------------------------------------
function cors(res) {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type');
  res.setHeader('Access-Control-Expose-Headers', 'Content-Disposition, Content-Length, Accept-Ranges');
}

function sendJson(res, status, obj) {
  const body = JSON.stringify(obj);
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Content-Length': Buffer.byteLength(body) });
  res.end(body);
}

function sendNoContent(res) {
  res.writeHead(204);
  res.end();
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    req.on('error', reject);
  });
}

function checkAuth(req) {
  const header = req.headers.authorization ?? '';
  const m = /^Bearer (.+)$/.exec(header);
  if (!m) return null;
  const t = tokens.get(m[1]);
  if (!t || t.expiresAt < Date.now()) return null;
  return users.find((u) => u.id === t.userId) ?? null;
}

const MIME_BY_EXT = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  mp4: 'video/mp4',
  webm: 'video/webm',
  avi: 'video/x-msvideo',
};

function serveBytes(req, res, buf, contentType, extra = {}) {
  res.writeHead(200, {
    'Content-Type': contentType,
    'Content-Length': buf.length,
    'Accept-Ranges': 'bytes',
    'Cache-Control': extra.cacheControl ?? 'private, max-age=3600',
    ...(extra.contentDisposition ? { 'Content-Disposition': extra.contentDisposition } : {}),
  });
  res.end(req.method === 'HEAD' ? undefined : buf);
}

function serveRange(req, res, buf, contentType, extra = {}) {
  const range = /^bytes=(\d*)-(\d*)$/.exec(String(req.headers.range ?? ''));
  if (!range) return serveBytes(req, res, buf, contentType, extra);
  let start = range[1] === '' ? 0 : Number(range[1]);
  let end = range[2] === '' ? buf.length - 1 : Number(range[2]);
  if (Number.isNaN(start) || Number.isNaN(end) || start > end || start >= buf.length) {
    res.writeHead(416, { 'Content-Range': `bytes */${buf.length}` });
    return res.end();
  }
  end = Math.min(end, buf.length - 1);
  const slice = buf.subarray(start, end + 1);
  res.writeHead(206, {
    'Content-Type': contentType,
    'Content-Length': slice.length,
    'Content-Range': `bytes ${start}-${end}/${buf.length}`,
    'Accept-Ranges': 'bytes',
    'Cache-Control': extra.cacheControl ?? 'private, max-age=3600',
    ...(extra.contentDisposition ? { 'Content-Disposition': extra.contentDisposition } : {}),
  });
  res.end(slice);
}

// Item list sorting per contract.
function sortItems(list, sort, order) {
  const dir = order === 'asc' ? 1 : -1;
  const sorted = [...list];
  sorted.sort((a, b) => {
    switch (sort) {
      case 'name':
        return a.name.localeCompare(b.name, undefined, { numeric: true }) * dir;
      case 'size':
        return (a.size - b.size) * dir;
      case 'type':
        // Photos before videos, then by name in the requested direction.
        if (a.type !== b.type) return a.type === 'photo' ? -1 : 1;
        return a.name.localeCompare(b.name, undefined, { numeric: true }) * dir;
      case 'date':
      default:
        return (Date.parse(a.modTime) - Date.parse(b.modTime)) * dir;
    }
  });
  return sorted;
}

function paginate(list, page, pageSize) {
  const size = Math.min(Math.max(pageSize, 1), 500);
  const p = Math.max(page, 1);
  return { slice: list.slice((p - 1) * size, p * size), page: p, pageSize: size, total: list.length };
}

function listParams(url) {
  const q = url.searchParams;
  return {
    sort: q.get('sort') ?? 'date',
    order: q.get('order') ?? 'desc',
    type: q.get('type') ?? 'all',
    page: Number(q.get('page') ?? 1),
    pageSize: Number(q.get('pageSize') ?? 100),
  };
}

function itemPageResponse(list, url, extra = {}) {
  const { sort, order, type, page, pageSize } = listParams(url);
  const filtered = list.filter((i) => (type === 'all' ? true : i.type === type));
  const sorted = sortItems(filtered, sort, order);
  const { slice, page: p, pageSize: s, total } = paginate(sorted, page, pageSize);
  return { items: slice.map(serializeItem), page: p, pageSize: s, total, ...extra };
}

// ---------------------------------------------------------------------------
// Library scan state
// ---------------------------------------------------------------------------
let scanningUntil = 0;
const lastScanAt = '2026-03-01T00:00:00Z';

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------
const server = http.createServer(async (req, res) => {
  cors(res);
  const url = new URL(req.url, `http://${req.headers.host ?? 'localhost'}`);
  const path = url.pathname.replace(/\/+$/, '') || '/';
  const method = req.method ?? 'GET';

  if (VERBOSE) console.log(method, path, url.search);

  if (method === 'OPTIONS') return sendNoContent(res);

  try {
    // ---- public ----
    if (method === 'GET' && path === '/api/health') {
      return sendJson(res, 200, { ok: true, version: VERSION });
    }

    if (method === 'POST' && path === '/api/auth/login') {
      const ip = req.socket.remoteAddress ?? '?';
      const body = JSON.parse((await readBody(req)) || '{}');
      const username = String(body.username ?? '');
      const password = String(body.password ?? '');
      const rememberMe = Boolean(body.rememberMe);

      const key = `${username}|${ip}`;
      const fail = loginFailures.get(key);
      if (fail && fail.lockedUntil > Date.now()) {
        res.setHeader('Retry-After', String(Math.ceil((fail.lockedUntil - Date.now()) / 1000)));
        return sendJson(res, 429, { error: 'Too many failed attempts. Try again later.' });
      }

      const user = users.find((u) => u.username === username && u.password === password);
      if (!user) {
        const count = (fail?.count ?? 0) + 1;
        const lock = count >= 5 ? Math.min(60 * 2 ** (count - 5), 900) * 1000 : 0;
        loginFailures.set(key, { count, lockedUntil: lock ? Date.now() + lock : 0 });
        return sendJson(res, 401, { error: 'Invalid username or password' });
      }

      loginFailures.delete(key);
      const token = crypto.randomBytes(32).toString('hex');
      const ttl = rememberMe ? 180 : 30;
      const expiresAt = new Date(Date.now() + ttl * 24 * 60 * 60 * 1000).toISOString();
      tokens.set(token, { userId: user.id, expiresAt: Date.now() + ttl * 24 * 60 * 60 * 1000 });
      return sendJson(res, 200, { token, expiresAt });
    }

    // ---- authed JSON ----
    const user = checkAuth(req);
    const isAuthedJsonPath =
      path === '/api/me' ||
      path === '/api/auth/logout' ||
      path === '/api/favorites' ||
      path === '/api/search' ||
      path.startsWith('/api/folders') ||
      path.startsWith('/api/items') ||
      path.startsWith('/api/library') ||
      path.startsWith('/api/auth/');
    if (isAuthedJsonPath && !user) {
      return sendJson(res, 401, { error: 'Missing or invalid token' });
    }

    if (method === 'POST' && path === '/api/auth/logout') {
      const m = /^Bearer (.+)$/.exec(String(req.headers.authorization ?? ''));
      if (m) tokens.delete(m[1]);
      return sendNoContent(res);
    }

    if (method === 'GET' && path === '/api/me') {
      return sendJson(res, 200, { user: { id: user.id, username: user.username }, server: { name: 'Mock NAS', version: VERSION } });
    }

    let m;
    if ((m = /^\/api\/folders\/([^/]+)$/.exec(path)) && method === 'GET') {
      const id = decodeURIComponent(m[1]);
      if (id === 'root') {
        return sendJson(res, 200, {
          folder: null,
          children: folderDefs
            .filter((f) => f.parentId === 'root')
            .map(serializeFolder)
            .sort((a, b) => a.name.localeCompare(b.name)),
          breadcrumbs: [],
        });
      }
      const f = folderById.get(id);
      if (!f) return sendJson(res, 404, { error: 'Folder not found' });
      // Walk up to the virtual root; breadcrumbs include root and the folder itself.
      const crumbs = [serializeFolder(ROOT)];
      const chain = [];
      let cur = f;
      while (cur) {
        chain.unshift(cur);
        cur = folderById.get(cur.parentId ?? '');
      }
      for (const c of chain) crumbs.push(serializeFolder(c));
      return sendJson(res, 200, {
        folder: serializeFolder(f),
        children: folderDefs
          .filter((c) => c.parentId === f.id)
          .map(serializeFolder)
          .sort((a, b) => a.name.localeCompare(b.name)),
        breadcrumbs: crumbs,
      });
    }

    if ((m = /^\/api\/folders\/([^/]+)\/items$/.exec(path)) && method === 'GET') {
      const id = decodeURIComponent(m[1]);
      const list = id === 'root' ? [] : (itemsByFolder.get(id) ?? null);
      if (list === null) return sendJson(res, 404, { error: 'Folder not found' });
      const folder = id === 'root' ? null : serializeFolder(folderById.get(id));
      return sendJson(res, 200, itemPageResponse(list, url, { folder }));
    }

    if ((m = /^\/api\/items\/([^/]+)$/.exec(path)) && method === 'GET') {
      const item = items.find((i) => i.id === decodeURIComponent(m[1]));
      if (!item) return sendJson(res, 404, { error: 'Item not found' });
      return sendJson(res, 200, serializeItem(item));
    }

    if ((m = /^\/api\/items\/([^/]+)\/favorite$/.exec(path)) && method === 'PUT') {
      const item = items.find((i) => i.id === decodeURIComponent(m[1]));
      if (!item) return sendJson(res, 404, { error: 'Item not found' });
      const body = JSON.parse((await readBody(req)) || '{}');
      const fav = Boolean(body.favorite);
      const set = favoritesByUser.get(user.id) ?? new Set();
      if (fav) set.add(item.id);
      else set.delete(item.id);
      favoritesByUser.set(user.id, set);
      return sendJson(res, 200, { favorite: fav });
    }

    if (method === 'GET' && path === '/api/favorites') {
      const set = favoritesByUser.get(user.id) ?? new Set();
      const list = items.filter((i) => set.has(i.id));
      return sendJson(res, 200, itemPageResponse(list, url));
    }

    if (method === 'GET' && path === '/api/search') {
      const q = (url.searchParams.get('q') ?? '').trim();
      if (q.length < 1) return sendJson(res, 400, { error: 'Query must be at least 1 character' });
      const needle = q.toLowerCase();
      const list = items.filter((i) => i.name.toLowerCase().includes(needle));
      return sendJson(res, 200, itemPageResponse(list, url));
    }

    if (method === 'POST' && path === '/api/library/scan') {
      scanningUntil = Date.now() + 3000; // simulate a short scan
      return sendJson(res, 200, { scanning: true });
    }

    if (method === 'GET' && path === '/api/library/status') {
      const counts = { photo: items.filter((i) => i.type === 'photo').length, video: items.filter((i) => i.type === 'video').length };
      return sendJson(res, 200, { scanning: Date.now() < scanningUntil, lastScanAt, counts });
    }

    // ---- media (signed URLs, no Bearer) ----
    if ((m = /^\/api\/media\/([^/]+)\/thumb$/.exec(path)) && method === 'GET') {
      const item = items.find((i) => i.id === decodeURIComponent(m[1]));
      if (!item) return sendJson(res, 404, { error: 'Item not found' });
      const exp = Number(url.searchParams.get('exp') ?? 0);
      const sig = String(url.searchParams.get('sig') ?? '');
      const variant = url.searchParams.get('s') ?? 'grid';
      const extra = `s=${variant}`;
      const want = crypto.createHmac('sha256', SECRET).update(`${item.id}|thumb|${extra}|${exp}`).digest('hex');
      if (exp < Math.floor(Date.now() / 1000) || sig !== want) {
        return sendJson(res, 401, { error: 'Invalid or expired signature' });
      }
      const buf = item.type === 'video' ? item.poster : item.bytes;
      return serveBytes(req, res, buf, 'image/png', { cacheControl: 'private, max-age=86400' });
    }

    if ((m = /^\/api\/media\/([^/]+)\/stream$/.exec(path)) && (method === 'GET' || method === 'HEAD')) {
      const item = items.find((i) => i.id === decodeURIComponent(m[1]));
      if (!item) return sendJson(res, 404, { error: 'Item not found' });
      const exp = Number(url.searchParams.get('exp') ?? 0);
      const sig = String(url.searchParams.get('sig') ?? '');
      const want = crypto.createHmac('sha256', SECRET).update(`${item.id}|stream||${exp}`).digest('hex');
      if (exp < Math.floor(Date.now() / 1000) || sig !== want) {
        return sendJson(res, 401, { error: 'Invalid or expired signature' });
      }
      const extra = {};
      if (url.searchParams.get('download') === '1') {
        extra.contentDisposition = `attachment; filename*=UTF-8''${encodeURIComponent(item.name)}`;
      }
      return serveRange(req, res, item.bytes, item.mime, extra);
    }

    return sendJson(res, 404, { error: 'Not found' });
  } catch (err) {
    console.error('mock server error:', err);
    return sendJson(res, 500, { error: 'Internal error' });
  }
});

server.listen(PORT, () => {
  console.log(`nas-gallery mock API listening on http://127.0.0.1:${PORT} (demo/demo)`);
});

for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => {
    server.close(() => process.exit(0));
    setTimeout(() => process.exit(0), 500).unref();
  });
}
