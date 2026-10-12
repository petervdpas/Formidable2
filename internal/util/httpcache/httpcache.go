// Package httpcache makes a response for mutable content revalidate on every
// use, so a client never keeps showing an old copy of a file that changed
// under the same URL.
package httpcache

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// NotModified sets Cache-Control: no-cache and an ETag derived from body, and
// answers 304 when the request's If-None-Match already names that ETag. It
// reports whether it did; if not, the caller writes the body as usual.
func NotModified(w http.ResponseWriter, r *http.Request, body []byte) bool {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", etag)
	for _, tag := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == etag || tag == "*" {
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}
	return false
}
