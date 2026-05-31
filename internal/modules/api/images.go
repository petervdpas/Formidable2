package api

import (
	"encoding/base64"
	"net/http"
	"strconv"
)

// imageBytes serves GET /api/images/{tpl}/{filename}: raw image bytes, or the data: URL string
// with ?format=url. Does not require collection mode; traversal is rejected by the storage helper.
func (h *Handler) imageBytes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		w.Header().Set("Allow", "GET, HEAD")
		writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}

	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusBadRequest, "bad-template")
		return
	}
	filename := r.PathValue("filename")
	if filename == "" {
		writeJSONError(w, http.StatusBadRequest, "missing-filename")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "raw"
	}
	if format != "raw" && format != "url" {
		writeJSONError(w, http.StatusBadRequest, "bad-format")
		return
	}

	bytes, mime, err := h.st.OpenImageFile(stem+".yaml", filename)
	if err != nil {
		// Empty/traversal: 400 signals a malformed URL, not bad data.
		writeJSONError(w, http.StatusBadRequest, "bad-filename")
		return
	}
	if bytes == nil {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	switch format {
	case "url":
		body := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(bytes)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(body))
	default:
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(bytes)
	}
}
