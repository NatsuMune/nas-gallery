package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type testEnv struct {
	t       *testing.T
	dir     string
	media   string
	cfg     Config
	store   *Store
	scanner *Scanner
	app     *App
	srv     *httptest.Server
}

func newTestEnv(t *testing.T, populate func(mediaDir string)) *testEnv {
	t.Helper()
	dir := t.TempDir()
	media := filepath.Join(dir, "media")
	if err := os.MkdirAll(media, 0o755); err != nil {
		t.Fatal(err)
	}
	if populate != nil {
		populate(media)
	}
	cfg := defaultConfig()
	cfg.MediaPaths = []string{media}
	cfg.DataDir = filepath.Join(dir, "data")
	cfg.FFmpegPath = "/nonexistent-ffmpeg-binary-for-test"
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.thumbsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(cfg.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	secret, err := store.EnsureSecret()
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(cfg, store, cfg.thumbsDir())
	scanner.ScanSync()

	app := NewApp(cfg, store, secret, scanner)
	srv := httptest.NewServer(app.Router())
	t.Cleanup(srv.Close)

	return &testEnv{
		t:       t,
		dir:     dir,
		media:   media,
		cfg:     cfg,
		store:   store,
		scanner: scanner,
		app:     app,
		srv:     srv,
	}
}

func (e *testEnv) addUser(username, password string) int64 {
	e.t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		e.t.Fatal(err)
	}
	id, err := e.store.CreateUser(username, string(hash))
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *testEnv) login(username, password string, remember bool) string {
	e.t.Helper()
	body, _ := json.Marshal(map[string]any{
		"username":   username,
		"password":   password,
		"rememberMe": remember,
	})
	resp, err := http.Post(e.srv.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		e.t.Fatalf("login failed (%d): %s", resp.StatusCode, raw)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		e.t.Fatal(err)
	}
	return out.Token
}

func (e *testEnv) req(method, path, token string, body any) *http.Response {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, r)
	if err != nil {
		e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	return resp
}

func writeFixtureFile(t *testing.T, path string, data []byte, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}
