package handlers

import (
	"net/http"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

type CoverHandler struct {
	client *libgen.Client
}

func NewCoverHandler(client *libgen.Client) *CoverHandler {
	return &CoverHandler{client: client}
}

func (h *CoverHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	md5 := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("md5")))
	if !catalog.IsMD5(md5) {
		http.Error(w, "cover needs an md5 from the search results", http.StatusBadRequest)
		return
	}
	path, contentType, err := h.client.FetchCover(r.Context(), md5)
	if err != nil {
		writeCoverPlaceholder(w)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, path)
}

func writeCoverPlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(coverPlaceholderSVG)
}

// coverPlaceholderSVG is a 48×64 stand-in. Amarra CSP forbids img onerror JS,
// so the handler always returns an image body.
var coverPlaceholderSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="48" height="64" viewBox="0 0 48 64"><rect width="48" height="64" fill="#14222a"/><rect x="8" y="10" width="32" height="44" fill="none" stroke="#c9a06a" stroke-width="1.5"/></svg>`)
