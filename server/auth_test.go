package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestHealthAndCORS(t *testing.T) {
	env := newTestEnv(t, nil)

	// Public health check without auth.
	resp, err := http.Get(env.srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected CORS Allow-Origin *, got %q", got)
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Version != Version {
		t.Fatalf("unexpected health body: %+v", body)
	}

	// Trailing slash normalization.
	resp2, err := http.Get(env.srv.URL + "/api/health/")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("trailing slash failed: %d", resp2.StatusCode)
	}

	// OPTIONS preflight returns 204 with full CORS headers.
	req, _ := http.NewRequest(http.MethodOptions, env.srv.URL+"/api/folders/root", nil)
	pre, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	pre.Body.Close()
	if pre.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight expected 204, got %d", pre.StatusCode)
	}
	if pre.Header.Get("Access-Control-Allow-Methods") == "" || pre.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Fatalf("missing CORS headers on preflight: %+v", pre.Header)
	}
}

func TestAuthFlowAndRevocation(t *testing.T) {
	env := newTestEnv(t, nil)
	env.addUser("alice", "correct-horse-battery")

	// Unauthenticated protected endpoint returns 401.
	r := env.req(http.MethodGet, "/api/me", "", nil)
	r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", r.StatusCode)
	}

	// Unknown user and wrong password return identical 401 message.
	r1 := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "nobody", "password": "x"})
	var err1 map[string]string
	_ = json.NewDecoder(r1.Body).Decode(&err1)
	r1.Body.Close()

	r2 := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "alice", "password": "wrong"})
	var err2 map[string]string
	_ = json.NewDecoder(r2.Body).Decode(&err2)
	r2.Body.Close()

	if r1.StatusCode != 401 || r2.StatusCode != 401 || err1["error"] != err2["error"] {
		t.Fatalf("expected identical 401 error, got %d %v vs %d %v", r1.StatusCode, err1, r2.StatusCode, err2)
	}

	// Login with rememberMe=false vs true gives ~30d vs ~180d expiry.
	rShort := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{
		"username": "alice", "password": "correct-horse-battery", "rememberMe": false,
	})
	var shortRes struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	_ = json.NewDecoder(rShort.Body).Decode(&shortRes)
	rShort.Body.Close()
	if rShort.StatusCode != 200 || len(shortRes.Token) != 64 {
		t.Fatalf("bad login response: %d %+v", rShort.StatusCode, shortRes)
	}
	if d := time.Until(shortRes.ExpiresAt); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("expected ~30d TTL, got %v", d)
	}

	rLong := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{
		"username": "alice", "password": "correct-horse-battery", "rememberMe": true,
	})
	var longRes struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	_ = json.NewDecoder(rLong.Body).Decode(&longRes)
	rLong.Body.Close()
	if d := time.Until(longRes.ExpiresAt); d < 179*24*time.Hour || d > 181*24*time.Hour {
		t.Fatalf("expected ~180d TTL, got %v", d)
	}

	// /api/me works with valid token.
	meResp := env.req(http.MethodGet, "/api/me", shortRes.Token, nil)
	var me struct {
		User struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
		Server struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"server"`
	}
	_ = json.NewDecoder(meResp.Body).Decode(&me)
	meResp.Body.Close()
	if meResp.StatusCode != 200 || me.User.Username != "alice" || me.Server.Name != "My NAS" {
		t.Fatalf("unexpected /api/me: %d %+v", meResp.StatusCode, me)
	}

	// Logout revokes only that token.
	out := env.req(http.MethodPost, "/api/auth/logout", shortRes.Token, nil)
	out.Body.Close()
	if out.StatusCode != http.StatusNoContent {
		t.Fatalf("logout expected 204, got %d", out.StatusCode)
	}

	revoked := env.req(http.MethodGet, "/api/me", shortRes.Token, nil)
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected revoked token to return 401, got %d", revoked.StatusCode)
	}

	stillValid := env.req(http.MethodGet, "/api/me", longRes.Token, nil)
	stillValid.Body.Close()
	if stillValid.StatusCode != http.StatusOK {
		t.Fatalf("expected second token to remain valid, got %d", stillValid.StatusCode)
	}
}

func TestLoginRateLimiting(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	env := newTestEnv(t, nil)
	env.app.limiter = newLoginLimiter(func() time.Time { return now })
	env.addUser("bob", "secret")

	// 4 failures: not locked yet (all 401). Threshold is 5.
	for i := 0; i < 4; i++ {
		r := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "bob", "password": "bad"})
		r.Body.Close()
		if r.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, r.StatusCode)
		}
	}

	// 5th failure triggers 60s lock (still returns 401 for this failure itself).
	r5 := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "bob", "password": "bad"})
	r5.Body.Close()
	if r5.StatusCode != http.StatusUnauthorized {
		t.Fatalf("5th fail expected 401, got %d", r5.StatusCode)
	}

	// Subsequent attempt during lockout (even with valid password) -> 429 + Retry-After: 60.
	rLocked := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "bob", "password": "secret"})
	rLocked.Body.Close()
	if rLocked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 during lock, got %d", rLocked.StatusCode)
	}
	if ra, _ := strconv.Atoi(rLocked.Header.Get("Retry-After")); ra != 60 {
		t.Fatalf("expected Retry-After=60, got %q", rLocked.Header.Get("Retry-After"))
	}

	// Advance past 60s; 6th failure doubles lockout to 120s.
	now = now.Add(61 * time.Second)
	r6 := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "bob", "password": "bad"})
	r6.Body.Close()
	if r6.StatusCode != http.StatusUnauthorized {
		t.Fatalf("6th fail expected 401, got %d", r6.StatusCode)
	}
	rLocked2 := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "bob", "password": "secret"})
	rLocked2.Body.Close()
	if rLocked2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rLocked2.StatusCode)
	}
	if ra, _ := strconv.Atoi(rLocked2.Header.Get("Retry-After")); ra != 120 {
		t.Fatalf("expected doubled Retry-After=120, got %d", ra)
	}

	// After expiry, a good login succeeds and clears the failure counter.
	now = now.Add(121 * time.Second)
	b, _ := json.Marshal(map[string]any{"username": "bob", "password": "secret"})
	okResp, err := http.Post(env.srv.URL+"/api/auth/login", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after lock expired, got %d", okResp.StatusCode)
	}

	// Counter was reset: 1 bad attempt does NOT immediately lock.
	afterReset := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "bob", "password": "bad"})
	afterReset.Body.Close()
	if afterReset.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", afterReset.StatusCode)
	}
	stillAllowed := env.req(http.MethodPost, "/api/auth/login", "", map[string]any{"username": "bob", "password": "secret"})
	stillAllowed.Body.Close()
	if stillAllowed.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (not locked after reset), got %d", stillAllowed.StatusCode)
	}
}
