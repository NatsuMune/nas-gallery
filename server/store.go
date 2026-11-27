package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// schemaVersion is the current DB schema version stored in meta.
const schemaVersion = "1"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL COLLATE NOCASE UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tokens (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS folders (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	path      TEXT NOT NULL UNIQUE,
	name      TEXT NOT NULL,
	parent_id INTEGER REFERENCES folders(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS items (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	folder_id   INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
	path        TEXT NOT NULL UNIQUE,
	name        TEXT NOT NULL,
	ext         TEXT NOT NULL,
	type        TEXT NOT NULL CHECK (type IN ('photo','video')),
	size        INTEGER NOT NULL,
	mtime       TEXT NOT NULL,
	mtime_ns    INTEGER NOT NULL,
	width       INTEGER,
	height      INTEGER,
	duration_ms INTEGER
);
CREATE TABLE IF NOT EXISTS favorites (
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	item_id    INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	PRIMARY KEY (user_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_items_folder ON items(folder_id);
CREATE INDEX IF NOT EXISTS idx_items_type ON items(type);
CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id);
CREATE INDEX IF NOT EXISTS idx_favorites_user ON favorites(user_id);
`

// Store wraps the SQLite database holding users, tokens, the indexed library
// tree, favorites and meta values.
type Store struct {
	db *sql.DB
}

// OpenStore opens (creating if necessary) the SQLite database at dbPath with
// WAL mode, foreign keys and a busy timeout enabled, then applies migrations.
func OpenStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolve db path: %w", err)
	}
	dsn := buildDSN(abs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(8)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func buildDSN(absPath string) string {
	u := url.URL{Scheme: "file", Path: absPath}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?)`, schemaVersion); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	case err != nil:
		return fmt.Errorf("read schema version: %w", err)
	case v != schemaVersion:
		return fmt.Errorf("database schema version %q is newer/older than supported %q", v, schemaVersion)
	}
	return nil
}

// --- meta ---

func (s *Store) metaGet(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("meta get %q: %w", key, err)
	}
	return v, true, nil
}

func (s *Store) metaSet(key, value string) error {
	if _, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		return fmt.Errorf("meta set %q: %w", key, err)
	}
	return nil
}

// EnsureSecret returns the persisted random HMAC secret, creating it on first
// use. The secret signs media URLs.
func (s *Store) EnsureSecret() ([]byte, error) {
	if v, ok, err := s.metaGet("hmac_secret"); err != nil {
		return nil, err
	} else if ok {
		b, err := hex.DecodeString(v)
		if err == nil && len(b) == 32 {
			return b, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	if err := s.metaSet("hmac_secret", hex.EncodeToString(raw)); err != nil {
		return nil, err
	}
	return raw, nil
}

// LastScanAt returns the completion time of the last successful scan.
func (s *Store) LastScanAt() (time.Time, bool, error) {
	v, ok, err := s.metaGet("last_scan_at")
	if err != nil || !ok {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

func (s *Store) setLastScanAt(t time.Time) error {
	return s.metaSet("last_scan_at", t.UTC().Format(time.RFC3339))
}

// --- users ---

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// ErrUserExists is returned by CreateUser when the username is taken.
var ErrUserExists = errors.New("user already exists")

func (s *Store) CreateUser(username, passwordHash string) (int64, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, fmt.Errorf("username must not be empty")
	}
	res, err := s.db.Exec(`INSERT INTO users(username, password_hash, created_at) VALUES(?, ?, ?)`,
		username, passwordHash, nowRFC3339())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, fmt.Errorf("%w: %q", ErrUserExists, username)
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) UserByUsername(username string) (*User, error) {
	return s.queryUser(`SELECT id, username, password_hash, created_at FROM users WHERE username = ? COLLATE NOCASE`, username)
}

func (s *Store) UserByID(id int64) (*User, error) {
	return s.queryUser(`SELECT id, username, password_hash, created_at FROM users WHERE id = ?`, id)
}

func (s *Store) queryUser(q string, arg any) (*User, error) {
	var u User
	var created string
	err := s.db.QueryRow(q, arg).Scan(&u.ID, &u.Username, &u.PasswordHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &u, nil
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, password_hash, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var created string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &created); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteUser removes a user and cascades tokens and favorites.
func (s *Store) DeleteUser(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// --- tokens ---

// hashToken returns the SHA-256 hex digest of a raw token. Tokens are stored
// hashed so a leaked database cannot be replayed against the API.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateToken issues a new random 64-hex-char bearer token for userID with the
// given lifetime and returns the raw token plus its expiry.
func (s *Store) CreateToken(userID int64, ttl time.Duration) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().Add(ttl)
	_, err := s.db.Exec(`INSERT INTO tokens(user_id, token_hash, created_at, expires_at) VALUES(?, ?, ?, ?)`,
		userID, hashToken(token), nowRFC3339(), expires.UTC().Format(time.RFC3339))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("store token: %w", err)
	}
	// Opportunistic cleanup of expired sessions.
	go func() {
		_, _ = s.db.Exec(`DELETE FROM tokens WHERE expires_at < ?`, nowRFC3339())
	}()
	return token, expires, nil
}

// UserForToken resolves a raw bearer token to its user, or nil when the token
// is unknown or expired.
func (s *Store) UserForToken(token string) (*User, error) {
	var u User
	var created, expires string
	err := s.db.QueryRow(`
		SELECT u.id, u.username, u.created_at, t.expires_at
		FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ?`, hashToken(token)).Scan(&u.ID, &u.Username, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query token: %w", err)
	}
	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil || !exp.After(time.Now()) {
		// Expired: revoke so the row does not linger.
		_, _ = s.db.Exec(`DELETE FROM tokens WHERE token_hash = ?`, hashToken(token))
		return nil, nil
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &u, nil
}

// RevokeToken deletes the presented token (logout).
func (s *Store) RevokeToken(token string) error {
	if _, err := s.db.Exec(`DELETE FROM tokens WHERE token_hash = ?`, hashToken(token)); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

// --- folders ---

type folderRow struct {
	ID       int64
	Path     string
	Name     string
	ParentID sql.NullInt64
}

func (s *Store) folderByID(id int64) (*folderRow, error) {
	var f folderRow
	err := s.db.QueryRow(`SELECT id, path, name, parent_id FROM folders WHERE id = ?`, id).
		Scan(&f.ID, &f.Path, &f.Name, &f.ParentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query folder: %w", err)
	}
	return &f, nil
}

// rootFolders returns the media-root folders (parent IS NULL), name-sorted.
func (s *Store) rootFolders() ([]folderRow, error) {
	return s.queryFolders(`SELECT id, path, name, parent_id FROM folders WHERE parent_id IS NULL ORDER BY name COLLATE NOCASE, id`)
}

func (s *Store) childFolders(parentID int64) ([]folderRow, error) {
	return s.queryFolders(`SELECT id, path, name, parent_id FROM folders WHERE parent_id = ? ORDER BY name COLLATE NOCASE, id`, parentID)
}

func (s *Store) queryFolders(q string, args ...any) ([]folderRow, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()
	var out []folderRow
	for rows.Next() {
		var f folderRow
		if err := rows.Scan(&f.ID, &f.Path, &f.Name, &f.ParentID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// folderCounts returns (fileCount, childCount) per folder id for the given ids.
func (s *Store) folderCounts(ctx context.Context, ids []int64) (map[int64]int, map[int64]int, error) {
	files := make(map[int64]int, len(ids))
	children := make(map[int64]int, len(ids))
	if len(ids) == 0 {
		return files, children, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT folder_id, COUNT(*) FROM items WHERE folder_id IN (`+placeholders+`) GROUP BY folder_id`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("count items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, nil, err
		}
		files[id] = int(n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows2, err := s.db.QueryContext(ctx, `SELECT parent_id, COUNT(*) FROM folders WHERE parent_id IN (`+placeholders+`) GROUP BY parent_id`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("count folders: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var id, n int64
		if err := rows2.Scan(&id, &n); err != nil {
			return nil, nil, err
		}
		children[id] = int(n)
	}
	return files, children, rows2.Err()
}

// coverItemIDs picks, per folder, the id of the newest photo (preferred) or
// newest video (fallback) — the folder tile art. Missing folders map to no entry.
func (s *Store) coverItemIDs(ctx context.Context, ids []int64) (map[int64]int64, error) {
	covers := make(map[int64]int64, len(ids))
	for _, id := range ids {
		var itemID int64
		err := s.db.QueryRowContext(ctx, `
			SELECT id FROM items WHERE folder_id = ?
			ORDER BY (CASE type WHEN 'photo' THEN 0 ELSE 1 END), mtime_ns DESC, id ASC
			LIMIT 1`, id).Scan(&itemID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("query cover: %w", err)
		}
		covers[id] = itemID
	}
	return covers, nil
}

// --- items ---

type itemRow struct {
	ID         int64
	FolderID   int64
	Path       string
	Name       string
	Ext        string
	Type       string
	Size       int64
	Mtime      time.Time
	Width      sql.NullInt64
	Height     sql.NullInt64
	DurationMs sql.NullInt64
}

// itemByID resolves an item by its internal id; nil when absent.
func (s *Store) itemByID(ctx context.Context, id int64) (*itemRow, error) {
	var it itemRow
	var mtime string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, folder_id, path, name, ext, type, size, mtime, width, height, duration_ms
		FROM items WHERE id = ?`, id).
		Scan(&it.ID, &it.FolderID, &it.Path, &it.Name, &it.Ext, &it.Type, &it.Size, &mtime, &it.Width, &it.Height, &it.DurationMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query item: %w", err)
	}
	it.Mtime, _ = time.Parse(time.RFC3339, mtime)
	return &it, nil
}

// listOpts describes item list queries (folder listing, favorites, search).
type listOpts struct {
	userID     int64 // whose favorite flags to compute
	folderID   int64 // items directly in this folder
	rootScope  bool  // items in top-level (media-root) folders — virtual root listing
	favoriteOf int64 // only items favorited by this user
	search     string
	typeFilter string // "", "photo", "video"
	sort       string // "name" | "date" | "size" | "type"
	order      string // "asc" | "desc"
	page       int
	pageSize   int
}

func (o listOpts) offset() int { return (o.page - 1) * o.pageSize }

func (s *Store) listItems(ctx context.Context, o listOpts) ([]itemRow, []bool /*favorite flags*/, int, error) {
	var where []string
	var args []any
	if o.rootScope {
		where = append(where, `folder_id IN (SELECT id FROM folders WHERE parent_id IS NULL)`)
	} else if o.folderID > 0 {
		where = append(where, `folder_id = ?`)
		args = append(args, o.folderID)
	}
	if o.favoriteOf > 0 {
		where = append(where, `id IN (SELECT item_id FROM favorites WHERE user_id = ?)`)
		args = append(args, o.favoriteOf)
	}
	if o.search != "" {
		where = append(where, `instr(lower(name), lower(?)) > 0`)
		args = append(args, o.search)
	}
	if o.typeFilter != "" {
		where = append(where, `type = ?`)
		args = append(args, o.typeFilter)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	// Count total.
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM items `+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, 0, fmt.Errorf("count items: %w", err)
	}

	orderSQL := itemOrderSQL(o.sort, o.order)
	q := `SELECT id, folder_id, path, name, ext, type, size, mtime, width, height, duration_ms,
		EXISTS(SELECT 1 FROM favorites f WHERE f.user_id = ? AND f.item_id = items.id)
		FROM items ` + whereSQL + ` ` + orderSQL + ` LIMIT ? OFFSET ?`
	args = append([]any{o.userID}, args...)
	args = append(args, o.pageSize, o.offset())

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var items []itemRow
	var favs []bool
	for rows.Next() {
		var it itemRow
		var mtime string
		var fav bool
		if err := rows.Scan(&it.ID, &it.FolderID, &it.Path, &it.Name, &it.Ext, &it.Type, &it.Size, &mtime, &it.Width, &it.Height, &it.DurationMs, &fav); err != nil {
			return nil, nil, 0, err
		}
		it.Mtime, _ = time.Parse(time.RFC3339, mtime)
		items = append(items, it)
		favs = append(favs, fav)
	}
	return items, favs, total, rows.Err()
}

// itemOrderSQL builds the ORDER BY clause. "type" sorts photos before videos,
// then by name; every sort has a stable id tie-breaker.
func itemOrderSQL(sort, order string) string {
	dir := "ASC"
	if order == "desc" {
		dir = "DESC"
	}
	switch sort {
	case "name":
		return "ORDER BY name COLLATE NOCASE " + dir + ", id ASC"
	case "size":
		return "ORDER BY size " + dir + ", id ASC"
	case "type":
		return "ORDER BY (CASE type WHEN 'photo' THEN 0 ELSE 1 END) " + dir + ", name COLLATE NOCASE " + dir + ", id ASC"
	default: // "date"
		return "ORDER BY mtime_ns " + dir + ", id ASC"
	}
}

// --- favorites ---

// SetFavorite marks or clears a favorite for user on item. It reports whether
// the item exists.
func (s *Store) SetFavorite(ctx context.Context, userID, itemID int64, fav bool) (bool, error) {
	if _, err := s.itemByID(ctx, itemID); err != nil {
		return false, err
	}
	if fav {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO favorites(user_id, item_id, created_at) VALUES(?, ?, ?)
			ON CONFLICT(user_id, item_id) DO NOTHING`, userID, itemID, nowRFC3339()); err != nil {
			return true, fmt.Errorf("set favorite: %w", err)
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND item_id = ?`, userID, itemID); err != nil {
			return true, fmt.Errorf("clear favorite: %w", err)
		}
	}
	return true, nil
}

func (s *Store) IsFavorite(ctx context.Context, userID, itemID int64) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM favorites WHERE user_id = ? AND item_id = ?`, userID, itemID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query favorite: %w", err)
	}
	return true, nil
}

// --- library status ---

func (s *Store) typeCounts(ctx context.Context) (photos, videos int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT type, COUNT(*) FROM items GROUP BY type`)
	if err != nil {
		return 0, 0, fmt.Errorf("count by type: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return 0, 0, err
		}
		switch t {
		case "photo":
			photos = n
		case "video":
			videos = n
		}
	}
	return photos, videos, rows.Err()
}

// --- scanner support ---

type snapItem struct {
	ID       int64
	FolderID int64
	Size     int64
	MtimeNS  int64
}

// itemsSnapshot loads the full path → (id, folder, size, mtime) index used by
// the scanner to decide what is new, changed or deleted.
func (s *Store) itemsSnapshot() (map[string]snapItem, error) {
	rows, err := s.db.Query(`SELECT id, folder_id, path, size, mtime_ns FROM items`)
	if err != nil {
		return nil, fmt.Errorf("snapshot items: %w", err)
	}
	defer rows.Close()
	out := make(map[string]snapItem)
	for rows.Next() {
		var p snapItem
		var path string
		if err := rows.Scan(&p.ID, &p.FolderID, &path, &p.Size, &p.MtimeNS); err != nil {
			return nil, err
		}
		out[path] = p
	}
	return out, rows.Err()
}

type folderUpsert struct {
	Path       string
	Name       string
	ParentPath string // "" = media root (virtual parent)
}

type itemUpsert struct {
	Path       string
	FolderPath string
	Name       string
	Ext        string
	Type       string
	Size       int64
	Mtime      time.Time
	Width      sql.NullInt64
	Height     sql.NullInt64
	DurationMs sql.NullInt64
}

type scanStats struct {
	FoldersAdded   int
	FoldersDeleted int
	ItemsAdded     int
	ItemsChanged   int
	ItemsDeleted   int
}

// ChangedItemIDs / DeletedItemIDs let the scanner invalidate cached thumbnails
// after the transaction commits.
type scanResult struct {
	Stats          scanStats
	ChangedItemIDs []int64
	DeletedItemIDs []int64
}

// applyScan applies one full incremental scan in a single transaction:
// folder upserts, item upserts (new + changed), deletions of everything not
// seen, and the last_scan_at meta value. It returns per-item change lists.
func (s *Store) applyScan(folders []folderUpsert, folderSeen map[string]bool,
	items []itemUpsert, itemSeen map[string]bool, snapshot map[string]snapItem) (scanResult, error) {

	var res scanResult
	tx, err := s.db.Begin()
	if err != nil {
		return res, fmt.Errorf("begin scan tx: %w", err)
	}
	defer tx.Rollback()

	// Map every known folder path to its row id (existing or just created).
	folderIDs := make(map[string]int64, len(folders))
	existing, err := s.queryFolderIDs(tx)
	if err != nil {
		return res, err
	}
	for path, id := range existing {
		folderIDs[path] = id
	}
	for _, fu := range folders {
		if _, ok := folderIDs[fu.Path]; ok {
			continue
		}
		var parent any
		if fu.ParentPath != "" {
			pid, ok := folderIDs[fu.ParentPath]
			if !ok {
				return res, fmt.Errorf("scan bug: parent folder %q not indexed before %q", fu.ParentPath, fu.Path)
			}
			parent = pid
		}
		r, err := tx.Exec(`INSERT INTO folders(path, name, parent_id) VALUES(?, ?, ?)`, fu.Path, fu.Name, parent)
		if err != nil {
			return res, fmt.Errorf("insert folder %q: %w", fu.Path, err)
		}
		id, err := r.LastInsertId()
		if err != nil {
			return res, err
		}
		folderIDs[fu.Path] = id
		res.Stats.FoldersAdded++
	}

	// Items: update changed rows, insert new ones.
	for _, iu := range items {
		folderID, ok := folderIDs[iu.FolderPath]
		if !ok {
			return res, fmt.Errorf("scan bug: folder %q missing for item %q", iu.FolderPath, iu.Path)
		}
		mtimeNS := iu.Mtime.UnixNano()
		mtimeStr := iu.Mtime.UTC().Format(time.RFC3339)
		if snap, exists := snapshot[iu.Path]; exists {
			if _, err := tx.Exec(`UPDATE items SET folder_id=?, name=?, ext=?, type=?, size=?, mtime=?, mtime_ns=?,
				width=?, height=?, duration_ms=? WHERE id=?`,
				folderID, iu.Name, iu.Ext, iu.Type, iu.Size, mtimeStr, mtimeNS,
				iu.Width, iu.Height, iu.DurationMs, snap.ID); err != nil {
				return res, fmt.Errorf("update item %q: %w", iu.Path, err)
			}
			res.Stats.ItemsChanged++
			res.ChangedItemIDs = append(res.ChangedItemIDs, snap.ID)
		} else {
			if _, err := tx.Exec(`INSERT INTO items(folder_id, path, name, ext, type, size, mtime, mtime_ns, width, height, duration_ms)
				VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				folderID, iu.Path, iu.Name, iu.Ext, iu.Type, iu.Size, mtimeStr, mtimeNS,
				iu.Width, iu.Height, iu.DurationMs); err != nil {
				return res, fmt.Errorf("insert item %q: %w", iu.Path, err)
			}
			res.Stats.ItemsAdded++
		}
	}

	// Deletions: anything indexed but not seen this walk.
	for path, snap := range snapshot {
		if itemSeen[path] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM items WHERE id = ?`, snap.ID); err != nil {
			return res, fmt.Errorf("delete item %q: %w", path, err)
		}
		res.Stats.ItemsDeleted++
		res.DeletedItemIDs = append(res.DeletedItemIDs, snap.ID)
	}
	for path, id := range existing {
		if folderSeen[path] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM folders WHERE id = ?`, id); err != nil {
			return res, fmt.Errorf("delete folder %q: %w", path, err)
		}
		res.Stats.FoldersDeleted++
	}

	if err := s.setLastScanAtTx(tx, time.Now()); err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("commit scan tx: %w", err)
	}
	return res, nil
}

func (s *Store) queryFolderIDs(q queryer) (map[string]int64, error) {
	rows, err := q.Query(`SELECT path, id FROM folders`)
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var path string
		var id int64
		if err := rows.Scan(&path, &id); err != nil {
			return nil, err
		}
		out[path] = id
	}
	return out, rows.Err()
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func (s *Store) setLastScanAtTx(tx *sql.Tx, t time.Time) error {
	if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES('last_scan_at', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, t.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("set last scan time: %w", err)
	}
	return nil
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
