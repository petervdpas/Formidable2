package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A diagram image keeps its URL while its content changes on every redraw, so
// the route must make clients revalidate and must serve new content at once.
func TestImageBytes_RevalidatesAndServesChangedContent(t *testing.T) {
	st := newStubStorage()
	h := NewHandler(newStub(), st, newStubWriter(), newStubTemplates(), nil, nil, nil)
	const path = "/api/images/datastroom/r.diagram_test.diagram.svg"
	get := func(etag string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		h.ServeHTTP(rec, req)
		return rec
	}

	st.putImage("datastroom.yaml", "r.diagram_test.diagram.svg", []byte("<svg>one</svg>"))
	first := get("")
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || first.Body.String() != "<svg>one</svg>" || etag == "" || first.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("first: code %d headers %v", first.Code, first.Header())
	}
	if again := get(etag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("unchanged image: code %d, want 304 with no body", again.Code)
	}

	st.putImage("datastroom.yaml", "r.diagram_test.diagram.svg", []byte("<svg>two</svg>"))
	changed := get(etag)
	if changed.Code != http.StatusOK || changed.Body.String() != "<svg>two</svg>" {
		t.Fatalf("redrawn image must be served despite the old validator: code %d body %q", changed.Code, changed.Body.String())
	}
	if changed.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("the SVG content-security policy must still be sent")
	}
}
