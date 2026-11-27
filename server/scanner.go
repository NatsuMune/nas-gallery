package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// excludedNames are skipped during the walk (both as files and directories).
var excludedNames = map[string]bool{
	"@eaDir":                    true,
	"#recycle":                  true,
	"#snapshot":                 true,
	"System Volume Information": true,
	"lost+found":                true,
	".DS_Store":                 true,
	"Thumbs.db":                 true,
}

// isExcludedName matches the contract's exclusion list: special NAS dirs plus
// all dotfiles/dot-dirs.
func isExcludedName(name string) bool {
	return excludedNames[name] || strings.HasPrefix(name, ".")
}

// photoExts / videoExts are the recognized media extensions.
var photoExts = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true,
	"bmp": true, "tiff": true, "avif": true, "heic": true, "heif": true,
}

var videoExts = map[string]bool{
	"mp4": true, "webm": true, "mov": true, "mkv": true, "m4v": true, "avi": true,
}

// mediaTypeOf returns "photo", "video" or "" (not recognized) for an extension
// (without the dot, lowercase).
func mediaTypeOf(ext string) (string, bool) {
	if photoExts[ext] {
		return "photo", true
	}
	if videoExts[ext] {
		return "video", true
	}
	return "", false
}

// mp4FamilyExts are containers we can parse natively for duration and (best
// effort) dimensions: the QuickTime-family moov/mvhd/tkhd boxes.
var mp4FamilyExts = map[string]bool{
	"mp4": true, "m4v": true, "mov": true,
}

// matroskaFamilyExts are EBML containers we parse for duration.
var matroskaFamilyExts = map[string]bool{
	"webm": true, "mkv": true,
}

// Scanner walks all configured media roots incrementally, indexing by
// (path, size, mtime), removing deleted entries, and refreshing metadata for
// changed files. Scans run at startup, every rescanIntervalHours and on
// POST /api/library/scan.
type Scanner struct {
	mu        sync.Mutex
	running   bool
	cfg       Config
	store     *Store
	thumbsDir string
}

func NewScanner(cfg Config, store *Store, thumbsDir string) *Scanner {
	return &Scanner{cfg: cfg, store: store, thumbsDir: thumbsDir}
}

// IsScanning reports whether a scan is currently in progress.
func (s *Scanner) IsScanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Trigger starts an asynchronous scan if none is running (idempotent).
func (s *Scanner) Trigger() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	go func() {
		defer s.finish()
		s.scanOnce()
	}()
}

// Run performs the initial scan and then rescans periodically until ctx ends.
func (s *Scanner) Run(ctx context.Context) {
	s.ScanSync()
	if s.cfg.RescanIntervalHour <= 0 {
		<-ctx.Done()
		return
	}
	t := time.NewTicker(time.Duration(s.cfg.RescanIntervalHour) * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.ScanSync()
		}
	}
}

// ScanSync runs a full scan (used at startup, by the ticker and by tests);
// it is a no-op if a scan is already in progress.
func (s *Scanner) ScanSync() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	defer s.finish()
	s.scanOnce()
}

func (s *Scanner) finish() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func (s *Scanner) scanOnce() {
	start := time.Now()
	folderSeen := make(map[string]bool)
	var folders []folderUpsert
	itemSeen := make(map[string]bool)
	var items []itemUpsert

	snapshot, err := s.store.itemsSnapshot()
	if err != nil {
		fmt.Printf("scan: cannot load snapshot: %v\n", err)
		return
	}

	for _, root := range s.cfg.MediaPaths {
		s.walkRoot(root, snapshot, &folders, folderSeen, &items, itemSeen)
	}

	res, err := s.store.applyScan(folders, folderSeen, items, itemSeen, snapshot)
	if err != nil {
		fmt.Printf("scan: failed to apply: %v\n", err)
		return
	}
	// Invalidate cached thumbnails for changed and deleted items.
	for _, id := range append(res.ChangedItemIDs, res.DeletedItemIDs...) {
		s.removeThumb(id)
	}
	fmt.Printf("scan: %d roots, +%d/~%d/-%d items, +%d/-%d folders, took %s\n",
		len(s.cfg.MediaPaths), res.Stats.ItemsAdded, res.Stats.ItemsChanged, res.Stats.ItemsDeleted,
		res.Stats.FoldersAdded, res.Stats.FoldersDeleted, time.Since(start).Round(time.Millisecond))
}

// walkRoot walks one media root, collecting folder and item upserts. The walk
// visits directories top-down, so parents are always collected before children.
func (s *Scanner) walkRoot(root string, snapshot map[string]snapItem,
	folders *[]folderUpsert, folderSeen map[string]bool,
	items *[]itemUpsert, itemSeen map[string]bool) {

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("scan: %v\n", err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		isRoot := path == root
		if !isRoot && isExcludedName(name) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			fu := folderUpsert{Path: path, Name: name}
			if !isRoot {
				fu.ParentPath = filepath.Dir(path)
			}
			*folders = append(*folders, fu)
			folderSeen[path] = true
			return nil
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
		mt, recognized := mediaTypeOf(ext)
		if !recognized {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			fmt.Printf("scan: stat %s: %v\n", path, err)
			return nil
		}
		itemSeen[path] = true
		if snap, exists := snapshot[path]; exists && snap.Size == info.Size() && snap.MtimeNS == info.ModTime().UnixNano() {
			return nil // unchanged
		}
		width, height, durationMs := extractItemMeta(path, ext, mt)
		*items = append(*items, itemUpsert{
			Path:       path,
			FolderPath: filepath.Dir(path),
			Name:       name,
			Ext:        ext,
			Type:       mt,
			Size:       info.Size(),
			Mtime:      info.ModTime(),
			Width:      nullInt(width),
			Height:     nullInt(height),
			DurationMs: nullInt64(int64(durationMs)),
		})
		return nil
	})
	if err != nil {
		fmt.Printf("scan: walk %s: %v\n", root, err)
	}
}

func (s *Scanner) removeThumb(itemID int64) {
	for _, v := range []string{"grid", "full"} {
		_ = os.Remove(filepath.Join(s.thumbsDir, fmt.Sprintf("%d_%s.jpg", itemID, v)))
	}
}

func nullInt(v int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(v), Valid: v > 0}
}

func nullInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: v > 0}
}
