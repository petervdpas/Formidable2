package httpcache

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func serve(body string, ifNoneMatch string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	if !NotModified(rec, req, []byte(body)) {
		_, _ = rec.Write([]byte(body))
	}
	return rec
}

func TestNotModified_FirstRequestGetsBodyAndValidators(t *testing.T) {
	rec := serve("<svg>one</svg>", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "<svg>one</svg>" {
		t.Fatalf("code %d body %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-cache" || rec.Header().Get("ETag") == "" {
		t.Fatalf("headers = %v, want revalidation on every use plus an ETag", rec.Header())
	}
}

func TestNotModified_SameContentAnswers304WithoutBody(t *testing.T) {
	etag := serve("<svg>one</svg>", "").Header().Get("ETag")
	rec := serve("<svg>one</svg>", etag)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("code %d body %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") != etag {
		t.Fatal("a 304 must repeat the ETag")
	}
}

func TestNotModified_ChangedContentIsServedAgain(t *testing.T) {
	old := serve("<svg>one</svg>", "").Header().Get("ETag")
	rec := serve("<svg>two</svg>", old)
	if rec.Code != http.StatusOK || rec.Body.String() != "<svg>two</svg>" || rec.Header().Get("ETag") == old {
		t.Fatalf("stale validator must not hide new content: code %d etag %q", rec.Code, rec.Header().Get("ETag"))
	}
}

func TestNotModified_MatchesListsWeakTagsAndWildcard(t *testing.T) {
	etag := serve("x", "").Header().Get("ETag")
	for _, h := range []string{`"other", ` + etag, "W/" + etag, "*"} {
		if rec := serve("x", h); rec.Code != http.StatusNotModified {
			t.Errorf("If-None-Match %q: code %d, want 304", h, rec.Code)
		}
	}
	if rec := serve("x", `"other"`); rec.Code != http.StatusOK {
		t.Errorf("unrelated tag: code %d, want 200", rec.Code)
	}
}

func TestNotModified_EmptyBodyStillHasAValidator(t *testing.T) {
	if serve("", "").Header().Get("ETag") == "" {
		t.Fatal("no ETag for empty body")
	}
}
