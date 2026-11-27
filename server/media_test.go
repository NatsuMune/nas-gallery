package main

import (
	"bytes"
	"encoding/json"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignedMediaRangeDownloadAndThumbs(t *testing.T) {
	t0 := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	var mp4Bytes []byte
	var pngBytes []byte

	env := newTestEnv(t, func(media string) {
		pngBytes = encodeTinyPNG(t, 800, 600)
		mp4Bytes = buildMinimalMP4(10000, 320, 180, 512)
		writeFixtureFile(t, filepath.Join(media, "photo one.png"), pngBytes, t0)
		writeFixtureFile(t, filepath.Join(media, "clip.mp4"), mp4Bytes, t0)
	})
	env.addUser("alice", "pw")
	tok := env.login("alice", "pw", false)

	// Search to get both items with signed URLs.
	sr := env.req(http.MethodGet, "/api/search?q=.", tok, nil)
	var list listResponseNoFolder
	_ = json.NewDecoder(sr.Body).Decode(&list)
	sr.Body.Close()

	var photo, video itemJSON
	for _, it := range list.Items {
		if it.Type == "photo" {
			photo = it
		} else {
			video = it
		}
	}

	// 1. Photo thumb (grid) generates cached JPEG and decodes cleanly.
	tr, err := http.Get(env.srv.URL + photo.ThumbURL)
	if err != nil {
		t.Fatal(err)
	}
	thumbBytes, _ := io.ReadAll(tr.Body)
	tr.Body.Close()
	if tr.StatusCode != 200 || tr.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("unexpected thumb response: %d %s", tr.StatusCode, tr.Header.Get("Content-Type"))
	}
	if tr.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS header on media thumb")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(thumbBytes))
	if err != nil {
		t.Fatalf("invalid JPEG thumb: %v", err)
	}
	if cfg.Width != 480 || cfg.Height != 360 {
		t.Fatalf("expected 480x360 thumb, got %dx%d", cfg.Width, cfg.Height)
	}

	// Video thumb without ffmpeg falls back to dark placeholder JPEG (always 200).
	vtr, err := http.Get(env.srv.URL + video.ThumbURL)
	if err != nil {
		t.Fatal(err)
	}
	vThumbBytes, _ := io.ReadAll(vtr.Body)
	vtr.Body.Close()
	if vtr.StatusCode != 200 || vtr.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("expected 200 JPEG placeholder for video, got %d", vtr.StatusCode)
	}
	if _, err := jpeg.DecodeConfig(bytes.NewReader(vThumbBytes)); err != nil {
		t.Fatalf("video placeholder not valid JPEG: %v", err)
	}

	// 2. Tampered signature / wrong variant / expired URL rejected with 401.
	badSig := strings.Replace(photo.ThumbURL, "s=grid", "s=full", 1)
	br, _ := http.Get(env.srv.URL + badSig)
	br.Body.Close()
	if br.StatusCode != 401 {
		t.Fatalf("expected 401 on swapped thumb variant, got %d", br.StatusCode)
	}

	tampered := photo.StreamURL + "0"
	trp, _ := http.Get(env.srv.URL + tampered)
	trp.Body.Close()
	if trp.StatusCode != 401 {
		t.Fatalf("expected 401 on tampered stream sig, got %d", trp.StatusCode)
	}

	expired := env.app.signMediaURL(photo.ID, "stream", url.Values{}, -10*time.Second)
	expResp, _ := http.Get(env.srv.URL + expired)
	expResp.Body.Close()
	if expResp.StatusCode != 401 {
		t.Fatalf("expected 401 on expired stream URL, got %d", expResp.StatusCode)
	}

	// 3. Range requests on streamUrl (206, Content-Range, suffix range, If-Range).
	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+video.StreamURL, nil)
	req.Header.Set("Range", "bytes=0-15")
	rngResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := io.ReadAll(rngResp.Body)
	rngResp.Body.Close()
	if rngResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rngResp.StatusCode)
	}
	if !bytes.Equal(part, mp4Bytes[:16]) {
		t.Fatalf("range bytes mismatch")
	}
	if got := rngResp.Header.Get("Content-Range"); !strings.HasPrefix(got, "bytes 0-15/") {
		t.Fatalf("unexpected Content-Range: %q", got)
	}
	if rngResp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("expected Accept-Ranges: bytes, got %q", rngResp.Header.Get("Accept-Ranges"))
	}

	// Suffix range.
	reqSuf, _ := http.NewRequest(http.MethodGet, env.srv.URL+video.StreamURL, nil)
	reqSuf.Header.Set("Range", "bytes=-12")
	sufResp, _ := http.DefaultClient.Do(reqSuf)
	sufBytes, _ := io.ReadAll(sufResp.Body)
	sufResp.Body.Close()
	if sufResp.StatusCode != http.StatusPartialContent || !bytes.Equal(sufBytes, mp4Bytes[len(mp4Bytes)-12:]) {
		t.Fatalf("suffix range mismatch: status=%d len=%d", sufResp.StatusCode, len(sufBytes))
	}

	// If-Range mismatch returns 200 full file instead of 206.
	reqIf, _ := http.NewRequest(http.MethodGet, env.srv.URL+video.StreamURL, nil)
	reqIf.Header.Set("Range", "bytes=0-15")
	reqIf.Header.Set("If-Range", `"stale-etag"`)
	ifResp, _ := http.DefaultClient.Do(reqIf)
	fullBytes, _ := io.ReadAll(ifResp.Body)
	ifResp.Body.Close()
	if ifResp.StatusCode != http.StatusOK || !bytes.Equal(fullBytes, mp4Bytes) {
		t.Fatalf("If-Range mismatch expected 200 full body, got %d len=%d", ifResp.StatusCode, len(fullBytes))
	}

	// 4. download=1 appended to signed streamUrl works and sets Content-Disposition.
	dlResp, err := http.Get(env.srv.URL + photo.StreamURL + "&download=1")
	if err != nil {
		t.Fatal(err)
	}
	dlBytes, _ := io.ReadAll(dlResp.Body)
	dlResp.Body.Close()
	if dlResp.StatusCode != 200 || !bytes.Equal(dlBytes, pngBytes) {
		t.Fatalf("download=1 failed: %d", dlResp.StatusCode)
	}
	if disp := dlResp.Header.Get("Content-Disposition"); !strings.Contains(disp, "attachment; filename*=UTF-8''photo%20one.png") {
		t.Fatalf("unexpected Content-Disposition: %q", disp)
	}

	// Cached thumb still exists on disk.
	idNum, _ := parseItemID(photo.ID)
	if _, err := os.Stat(env.app.thumbCachePath(idNum, "grid")); err != nil {
		t.Fatalf("expected cached thumb file on disk: %v", err)
	}
}
