package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Media URLs are signed with HMAC-SHA256 over
//
//	"<itemId>|<variant>|<extra-params-sorted>|<exp>"
//
// where variant is "stream" or "thumb" and extra-params is every query
// parameter of the URL except exp, sig, and download (clients append
// "&download=1" to signed streamUrls for attachment downloads), sorted by key
// (multiple values of one key are comma-joined in order), formatted "k=v" and
// joined with "&". For thumbs the extra param is s=grid|full.

func signMessage(itemID, variant, extra string, exp int64) string {
	return itemID + "|" + variant + "|" + extra + "|" + strconv.FormatInt(exp, 10)
}

func canonicalExtra(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		if k == "exp" || k == "sig" || k == "download" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(q[k], ","))
	}
	return strings.Join(parts, "&")
}

func (app *App) signMediaURL(itemID, variant string, extra url.Values, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	msg := signMessage(itemID, variant, canonicalExtra(extra), exp)
	mac := hmac.New(sha256.New, app.secret)
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))
	q := url.Values{}
	for k, v := range extra {
		for _, vv := range v {
			q.Add(k, vv)
		}
	}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", sig)
	return fmt.Sprintf("/api/media/%s/%s?%s", itemID, variant, q.Encode())
}

// verifySignedURL authenticates a media request. On failure it writes the 401
// JSON error itself and returns false.
func (app *App) verifySignedURL(w http.ResponseWriter, r *http.Request, itemID, variant string) bool {
	q := r.URL.Query()
	expStr, sig := q.Get("exp"), q.Get("sig")
	if expStr == "" || sig == "" {
		writeErr(w, http.StatusUnauthorized, "missing signature")
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid signature")
		return false
	}
	msg := signMessage(itemID, variant, canonicalExtra(q), exp)
	mac := hmac.New(sha256.New, app.secret)
	mac.Write([]byte(msg))
	want := mac.Sum(nil)
	got, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(want, got) {
		writeErr(w, http.StatusUnauthorized, "invalid signature")
		return false
	}
	if time.Now().Unix() > exp {
		writeErr(w, http.StatusUnauthorized, "signature expired")
		return false
	}
	return true
}
