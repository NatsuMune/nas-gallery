package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCmdInitWithFlagsAndPreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sub", "config.yaml")
	dataDir := filepath.Join(dir, "var-data")
	media1 := filepath.Join(dir, "photos")
	media2 := filepath.Join(dir, "videos")

	if err := cmdInit([]string{
		"-c", cfgPath,
		"-listen", "127.0.0.1:9090",
		"-server-name", "Living Room NAS: 1",
		"-media", media1 + ", " + media2,
		"-data-dir", dataDir,
	}); err != nil {
		t.Fatalf("cmdInit: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:9090" {
		t.Fatalf("ListenAddr = %q, want 127.0.0.1:9090", cfg.ListenAddr)
	}
	if cfg.ServerName != "Living Room NAS: 1" {
		t.Fatalf("ServerName = %q", cfg.ServerName)
	}
	if cfg.DataDir != dataDir {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, dataDir)
	}
	if len(cfg.MediaPaths) != 2 || cfg.MediaPaths[0] != media1 || cfg.MediaPaths[1] != media2 {
		t.Fatalf("MediaPaths = %v", cfg.MediaPaths)
	}

	// Re-running init (even with different flags) must leave config.yaml untouched.
	if err := cmdInit([]string{"-c", cfgPath, "-server-name", "Overwritten"}); err != nil {
		t.Fatalf("re-run cmdInit: %v", err)
	}
	cfg2, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig 2: %v", err)
	}
	if cfg2.ServerName != "Living Room NAS: 1" {
		t.Fatalf("existing config overwritten: ServerName = %q", cfg2.ServerName)
	}
}

func TestCmdUserPasswordEnv(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dataDir := filepath.Join(dir, "data")
	if err := cmdInit([]string{"-c", cfgPath, "-data-dir", dataDir}); err != nil {
		t.Fatalf("cmdInit: %v", err)
	}

	t.Setenv("TEST_USER_PW", "secret123")
	if err := cmdUser([]string{"add", "-c", cfgPath, "-password-env", "TEST_USER_PW", "zach"}); err != nil {
		t.Fatalf("cmdUser add: %v", err)
	}

	// Too-short password must be rejected.
	t.Setenv("TEST_SHORT_PW", "abc")
	if err := cmdUser([]string{"add", "-c", cfgPath, "-password-env", "TEST_SHORT_PW", "wife"}); err == nil {
		t.Fatal("expected error for short password")
	}

	// Verify user exists in DB and password checks out.
	store, err := OpenStore(filepath.Join(dataDir, "nas-gallery.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	u, err := store.UserByUsername("zach")
	if err != nil || u == nil {
		t.Fatalf("UserByUsername(zach) = %v, %v", u, err)
	}
	if err := os.Setenv("TEST_USER_PW", ""); err != nil {
		t.Fatal(err)
	}

	if err := cmdUser([]string{"rm", "-c", cfgPath, "zach"}); err != nil {
		t.Fatalf("cmdUser rm: %v", err)
	}
	u2, err := store.UserByUsername("zach")
	if err != nil || u2 != nil {
		t.Fatalf("expected zach removed, got %v, %v", u2, err)
	}
}
