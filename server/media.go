package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// contentTypeForExt maps recognized media extensions to MIME types.
var contentTypes = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
	"bmp":  "image/bmp",
	"tiff": "image/tiff",
	"avif": "image/avif",
	"heic": "image/heic",
	"heif": "image/heif",
	"mp4":  "video/mp4",
	"m4v":  "video/x-m4v",
	"webm": "video/webm",
	"mov":  "video/quicktime",
	"mkv":  "video/x-matroska",
	"avi":  "video/x-msvideo",
}

func contentTypeForExt(ext string) string {
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

// handleStream serves original media bytes via http.ServeContent, which gives
// us correct Range/206, Content-Range, If-Range and HEAD handling for free.
// Authentication is the HMAC-signed exp+sig query params.
func (app *App) handleStream(w http.ResponseWriter, r *http.Request, p params) {
	id := p["id"]
	if !app.verifySignedURL(w, r, id, "stream") {
		return
	}
	itemID, ok := parseItemID(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	it, err := app.store.itemByID(r.Context(), itemID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load item")
		return
	}
	if it == nil {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		// The file vanished since the last scan (or permissions changed).
		writeErr(w, http.StatusNotFound, "media file not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "cannot stat media file")
		return
	}

	w.Header().Set("Content-Type", contentTypeForExt(it.Ext))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(it.Name)))
	}
	// ServeContent: Accept-Ranges, 206 + Content-Range, If-Range, 416.
	http.ServeContent(w, r, filepath.Base(it.Path), st.ModTime(), f)
}

// handleThumb serves grid (long edge 480) / full (long edge 1280) JPEG
// thumbnails from the on-disk cache, generating them on demand. Undecodable
// photos and videos without ffmpeg get a dark placeholder JPEG (always 200).
func (app *App) handleThumb(w http.ResponseWriter, r *http.Request, p params) {
	id := p["id"]
	variant := r.URL.Query().Get("s")
	if variant != "grid" && variant != "full" {
		writeErr(w, http.StatusBadRequest, "s must be grid or full")
		return
	}
	if !app.verifySignedURL(w, r, id, "thumb") {
		return
	}
	itemID, ok := parseItemID(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	it, err := app.store.itemByID(r.Context(), itemID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to load item")
		return
	}
	if it == nil {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}

	cachePath := app.thumbCachePath(it.ID, variant)
	if serveThumbFile(w, r, cachePath) {
		return
	}
	// Generate once per item+variant; concurrent requests wait and re-read
	// the cache afterwards.
	app.gate.do(itemKey(itemID, variant), func() {
		app.generateThumb(it, variant, cachePath)
	})
	if serveThumbFile(w, r, cachePath) {
		return
	}
	app.servePlaceholder(w, variant)
}

func itemKey(id int64, variant string) string {
	return fmt.Sprintf("%d:%s", id, variant)
}

func (app *App) thumbCachePath(id int64, variant string) string {
	return filepath.Join(app.cfg.thumbsDir(), fmt.Sprintf("%d_%s.jpg", id, variant))
}

// serveThumbFile serves a cached thumbnail (or anything existing) with the
// documented cache headers. It reports whether the file existed.
func serveThumbFile(w http.ResponseWriter, r *http.Request, path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return false
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	return true
}

// servePlaceholder writes a generated dark JPEG (never 404, never cached on
// disk so a later-decodable file regenerates properly).
func (app *App) servePlaceholder(w http.ResponseWriter, variant string) {
	edge := app.cfg.ThumbFullEdge
	if variant == "grid" {
		edge = app.cfg.ThumbGridEdge
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(placeholderJPEG(edge))
}
