package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannerIncrementalAndExclusions(t *testing.T) {
	t0 := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	var env *testEnv
	env = newTestEnv(t, func(media string) {
		pngSmall := encodeTinyPNG(t, 100, 50)
		writeFixtureFile(t, filepath.Join(media, "Album", "keep.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, "Album", "notes.txt"), []byte("ignore me"), t0)
		// Excluded directories and files per contract.
		writeFixtureFile(t, filepath.Join(media, "Album", "@eaDir", "keep.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, "#recycle", "trashed.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, "#snapshot", "old.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, "System Volume Information", "sys.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, "lost+found", "orphan.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, ".hidden", "secret.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, "Album", ".dotfile.png"), pngSmall, t0)
		writeFixtureFile(t, filepath.Join(media, "Album", ".DS_Store"), []byte("x"), t0)
		writeFixtureFile(t, filepath.Join(media, "Album", "Thumbs.db"), []byte("x"), t0)
	})

	p, v, err := env.store.typeCounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if p != 1 || v != 0 {
		t.Fatalf("expected 1 photo and 0 videos after exclusions, got %d photos, %d videos", p, v)
	}

	items, _, total, err := env.store.listItems(t.Context(), listOpts{page: 1, pageSize: 50, sort: "name", order: "asc"})
	if err != nil || total != 1 || items[0].Name != "keep.png" {
		t.Fatalf("unexpected initial scan items: %+v err=%v", items, err)
	}
	if items[0].Width.Int64 != 100 || items[0].Height.Int64 != 50 {
		t.Fatalf("expected 100x50 dimensions, got %+v %+v", items[0].Width, items[0].Height)
	}

	// Modify keep.png (new dimensions & mtime) and add a new video.
	pngLarge := encodeTinyPNG(t, 200, 120)
	writeFixtureFile(t, filepath.Join(env.media, "Album", "keep.png"), pngLarge, t0.Add(time.Minute))
	writeFixtureFile(t, filepath.Join(env.media, "Album", "new.mp4"), buildMinimalMP4(5000, 640, 360, 32), t0)

	env.scanner.ScanSync()

	items, _, total, err = env.store.listItems(t.Context(), listOpts{page: 1, pageSize: 50, sort: "name", order: "asc"})
	if err != nil || total != 2 {
		t.Fatalf("expected 2 items after incremental add/update, got %d err=%v", total, err)
	}
	if items[0].Width.Int64 != 200 || items[0].Height.Int64 != 120 {
		t.Fatalf("expected updated dimensions 200x120, got %+v %+v", items[0].Width, items[0].Height)
	}
	if items[1].Name != "new.mp4" || items[1].DurationMs.Int64 != 5000 {
		t.Fatalf("expected new.mp4 with 5000ms duration, got %+v", items[1])
	}

	// Delete keep.png (plus fake a cached thumb to verify cleanup).
	thumbPath := env.app.thumbCachePath(items[0].ID, "grid")
	_ = os.WriteFile(thumbPath, []byte("cached"), 0o600)
	if err := os.Remove(filepath.Join(env.media, "Album", "keep.png")); err != nil {
		t.Fatal(err)
	}
	env.scanner.ScanSync()

	p, v, _ = env.store.typeCounts(t.Context())
	if p != 0 || v != 1 {
		t.Fatalf("expected 0 photos, 1 video after deletion, got %d/%d", p, v)
	}
	if _, err := os.Stat(thumbPath); !os.IsNotExist(err) {
		t.Fatalf("expected deleted item's cached thumb to be removed")
	}
}
