package main

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// --- login rate limiting -----------------------------------------------------
//
// 5 failed attempts per (username + client IP) lock the key out for 60 s,
// doubling per subsequent failure, capped at 15 minutes. A successful login
// clears the counter. Lockout attempts themselves are rejected before any
// credential check and do not extend the lock.

type loginAttempt struct {
	fails      int
	lockedUtil time.Time
}

type loginLimiter struct {
	mu  sync.Mutex
	m   map[string]*loginAttempt
	now func() time.Time
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{m: make(map[string]*loginAttempt), now: now}
}

const (
	loginFailThreshold = 5
	loginLockBase      = 60 * time.Second
	loginLockMax       = 15 * time.Minute
)

// lockedFor reports whether key is locked and, if so, how many seconds remain.
func (l *loginLimiter) lockedFor(key string) (int, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.m[key]
	if a == nil {
		return 0, false
	}
	remain := a.lockedUtil.Sub(l.now())
	if remain <= 0 {
		return 0, false
	}
	return int((remain + time.Second - 1) / time.Second), true
}

// recordFailure counts one failed login and (re)locks the key past the
// threshold. Returns the new lockout duration, 0 if none.
func (l *loginLimiter) recordFailure(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.m) > 100_000 { // memory guard; prunes expired entries
		now := l.now()
		for k, a := range l.m {
			if a.lockedUtil.Before(now) {
				delete(l.m, k)
			}
		}
	}
	a := l.m[key]
	if a == nil {
		a = &loginAttempt{}
		l.m[key] = a
	}
	a.fails++
	if a.fails < loginFailThreshold {
		return 0
	}
	lock := loginLockBase << uint(a.fails-loginFailThreshold)
	if lock > loginLockMax || lock <= 0 { // <= 0 guards shift overflow
		lock = loginLockMax
	}
	a.lockedUtil = l.now().Add(lock)
	return lock
}

// reset clears all failure state for key (successful login).
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, key)
}

func loginKey(username, ip string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "|" + ip
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- request authentication --------------------------------------------------

type ctxKey int

const ctxAuthUser ctxKey = iota

type authUser struct {
	ID       int64
	Username string
}

func withAuthUser(ctx context.Context, u *authUser) context.Context {
	return context.WithValue(ctx, ctxAuthUser, u)
}

func authUserFrom(ctx context.Context) *authUser {
	u, _ := ctx.Value(ctxAuthUser).(*authUser)
	return u
}

// authenticate validates the Bearer token and returns the user, writing a 401
// JSON error when the token is missing, invalid or expired.
func (app *App) authenticate(w http.ResponseWriter, r *http.Request) *authUser {
	token := bearerToken(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing bearer token")
		return nil
	}
	user, err := app.store.UserForToken(token)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "authentication failed")
		return nil
	}
	if user == nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired token")
		return nil
	}
	return &authUser{ID: user.ID, Username: user.Username}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// dummyHash equalizes timing for unknown usernames.
var dummyHash = func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("nas-gallery-dummy-password"), bcryptCost)
	return h
}()

// --- handlers ----------------------------------------------------------------

func (app *App) handleHealth(w http.ResponseWriter, r *http.Request, _ params) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": Version})
}

func (app *App) handleLogin(w http.ResponseWriter, r *http.Request, _ params) {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"rememberMe"`
	}
	if !readJSONBody(w, r, &body) {
		return
	}

	key := loginKey(body.Username, clientIP(r))
	if wait, locked := app.limiter.lockedFor(key); locked {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		writeErr(w, http.StatusTooManyRequests, "too many failed login attempts; retry later")
		return
	}

	user, err := app.store.UserByUsername(body.Username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	ok := false
	if user != nil {
		ok = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) == nil
	} else {
		// Burn comparable time so unknown users are indistinguishable.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(body.Password))
	}
	if !ok {
		app.limiter.recordFailure(key)
		writeErr(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	app.limiter.reset(key)
	token, expires, err := app.store.CreateToken(user.ID, time.Duration(app.cfg.tokenTtl(body.RememberMe))*24*time.Hour)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token":     token,
		"expiresAt": expires.UTC().Format(time.RFC3339),
	})
}

func (app *App) handleLogout(w http.ResponseWriter, r *http.Request, _ params) {
	token := bearerToken(r)
	if err := app.store.RevokeToken(token); err != nil {
		writeErr(w, http.StatusInternalServerError, "logout failed")
		return
	}
	writeNoContent(w)
}

func (app *App) handleMe(w http.ResponseWriter, r *http.Request, _ params) {
	u := authUserFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"user":   map[string]any{"id": u.ID, "username": u.Username},
		"server": map[string]any{"name": app.cfg.ServerName, "version": Version},
	})
}
