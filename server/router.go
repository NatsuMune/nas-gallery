package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// App wires the config, store, secret, scanner and rate limiter together and
// exposes the HTTP handler.
type App struct {
	cfg     Config
	store   *Store
	secret  []byte
	limiter *loginLimiter
	scanner *Scanner
	gate    *keyGate // singleflight for thumbnail generation
}

func NewApp(cfg Config, store *Store, secret []byte, scanner *Scanner) *App {
	return &App{
		cfg:     cfg,
		store:   store,
		secret:  secret,
		limiter: newLoginLimiter(time.Now),
		scanner: scanner,
		gate:    newKeyGate(),
	}
}

type route struct {
	method string
	segs   []string // e.g. ["api","folders","{id}"]
	auth   string   // "none" | "bearer" (media endpoints verify signatures internally)
	h      func(w http.ResponseWriter, r *http.Request, p params)
}

type params map[string]string

// Router builds the full /api handler including CORS, request logging, panic
// recovery and trailing-slash normalization.
func (app *App) Router() http.Handler {
	routes := []route{
		{method: "GET", segs: segs("/api/health"), auth: "none", h: app.handleHealth},
		{method: "POST", segs: segs("/api/auth/login"), auth: "none", h: app.handleLogin},
		{method: "POST", segs: segs("/api/auth/logout"), auth: "bearer", h: app.handleLogout},
		{method: "GET", segs: segs("/api/me"), auth: "bearer", h: app.handleMe},
		{method: "GET", segs: segs("/api/folders/{id}"), auth: "bearer", h: app.handleFolder},
		{method: "GET", segs: segs("/api/folders/{id}/items"), auth: "bearer", h: app.handleFolderItems},
		{method: "GET", segs: segs("/api/items/{id}"), auth: "bearer", h: app.handleItem},
		{method: "PUT", segs: segs("/api/items/{id}/favorite"), auth: "bearer", h: app.handleFavorite},
		{method: "GET", segs: segs("/api/favorites"), auth: "bearer", h: app.handleFavorites},
		{method: "GET", segs: segs("/api/search"), auth: "bearer", h: app.handleSearch},
		{method: "GET", segs: segs("/api/media/{id}/stream"), auth: "none", h: app.handleStream},
		{method: "GET", segs: segs("/api/media/{id}/thumb"), auth: "none", h: app.handleThumb},
		{method: "POST", segs: segs("/api/library/scan"), auth: "bearer", h: app.handleScan},
		{method: "GET", segs: segs("/api/library/status"), auth: "bearer", h: app.handleStatus},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.serve(w, r, routes)
	})
}

func segs(path string) []string { return strings.Split(strings.Trim(path, "/"), "/") }

func (app *App) serve(w http.ResponseWriter, r *http.Request, routes []route) {
	start := time.Now()
	w = &statusWriter{ResponseWriter: w, status: http.StatusOK}

	// Trailing slashes never matter.
	p := r.URL.Path
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
	}
	r.URL.Path = p

	setCORSHeaders(w)

	// OPTIONS preflight on any /api route → 204.
	if r.Method == http.MethodOptions && strings.HasPrefix(p, "/api") {
		w.WriteHeader(http.StatusNoContent)
		logAPI(r, http.StatusNoContent, start)
		return
	}

	defer func() {
		if rec := recover(); rec != nil {
			fmt.Printf("panic: %v (%s %s)\n", rec, r.Method, r.URL.Path)
			sw := w.(*statusWriter)
			if !sw.wrote {
				writeErr(w, http.StatusInternalServerError, "internal server error")
			}
		}
	}()

	reqSegs := segs(p)
	var matched *route
	var pmap params
	methodMismatch := false
	for i := range routes {
		rt := &routes[i]
		m, ok := matchSegs(rt.segs, reqSegs)
		if !ok {
			continue
		}
		if rt.method != r.Method && !(r.Method == http.MethodHead && rt.method == http.MethodGet) {
			methodMismatch = true
			continue
		}
		matched = rt
		pmap = m
		break
	}
	if matched == nil {
		if methodMismatch {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		} else {
			writeErr(w, http.StatusNotFound, "not found")
		}
		logAPI(r, statusOf(w), start)
		return
	}

	if matched.auth == "bearer" {
		user := app.authenticate(w, r)
		if user == nil {
			logAPI(r, statusOf(w), start) // authenticate wrote the 401
			return
		}
		r = r.WithContext(withAuthUser(r.Context(), user))
	}

	matched.h(w, r, pmap)
	logAPI(r, statusOf(w), start)
}

// statusWriter records the response status for request logging.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wrote {
		sw.status = code
		sw.wrote = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	sw.wrote = true
	return sw.ResponseWriter.Write(b)
}

func matchSegs(pattern, path []string) (params, bool) {
	if len(pattern) != len(path) {
		return nil, false
	}
	m := params{}
	for i, ps := range pattern {
		if strings.HasPrefix(ps, "{") && strings.HasSuffix(ps, "}") {
			if path[i] == "" {
				return nil, false
			}
			m[ps[1:len(ps)-1]] = path[i]
			continue
		}
		if ps != path[i] {
			return nil, false
		}
	}
	return m, true
}

// statusOf reports the status code written (best effort).
func statusOf(w http.ResponseWriter) int {
	if sw, ok := w.(*statusWriter); ok {
		return sw.status
	}
	return 200
}

func logAPI(r *http.Request, status int, start time.Time) {
	dur := time.Since(start).Round(time.Microsecond)
	fmt.Printf("%s %s %d %s\n", r.Method, r.URL.Path, status, dur)
}

func setCORSHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	h.Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Length, Accept-Ranges, Retry-After")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// readJSONBody decodes a JSON request body of at most 64 KiB into v.
func readJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
	if err != nil || len(body) > 64*1024 {
		writeErr(w, http.StatusBadRequest, "cannot read request body")
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}
