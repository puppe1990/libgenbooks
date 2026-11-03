package handlers

import (
	"net/http"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

// ZlibDownloadHandler resolves a book on Z-Library and 302s the browser to
// the signed CDN file — the reader's IP does the transfer, same philosophy as
// the libgen /download handshake.
type ZlibDownloadHandler struct {
	views   *view.Renderer
	site    meta.Site
	catalog *i18n.Catalog
	cfg     cais.Config
	books   *catalog.Store
	client  *zlibrary.Client
}

func NewZlibDownloadHandler(views *view.Renderer, site meta.Site, messages *i18n.Catalog, cfg cais.Config, books *catalog.Store, client *zlibrary.Client) *ZlibDownloadHandler {
	return &ZlibDownloadHandler{
		views:   views,
		site:    site,
		catalog: messages,
		cfg:     cfg,
		books:   books,
		client:  client,
	}
}

func (h *ZlibDownloadHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *ZlibDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	md5 := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("md5")))
	if !catalog.IsMD5(md5) {
		http.Error(w, h.t(r, "download.missing_md5"), http.StatusBadRequest)
		return
	}
	if h.client == nil {
		h.writeError(w, r, h.t(r, "download.zlib_disabled"))
		return
	}
	title := strings.TrimSpace(r.URL.Query().Get("title"))
	if title == "" {
		h.writeError(w, r, h.t(r, "download.zlib_no_title"))
		return
	}
	books, err := h.client.Search(r.Context(), title, 1)
	if err != nil {
		h.writeError(w, r, err.Error())
		return
	}
	match, ok := zlibrary.PickBook(books, title, strings.TrimSpace(r.URL.Query().Get("ext")), md5)
	if !ok {
		h.writeError(w, r, h.t(r, "download.zlib_no_copy"))
		return
	}
	link, err := h.client.DownloadLink(r.Context(), match)
	if err != nil {
		h.writeError(w, r, err.Error())
		return
	}
	rememberDownloadClick(h.books, r, md5, false)
	http.Redirect(w, r, link, http.StatusFound)
}

func (h *ZlibDownloadHandler) writeError(w http.ResponseWriter, r *http.Request, cause string) {
	writeView(w, r, h.views, h.cfg, "app", "download", amarraData(r, h.site, map[string]any{
		"Title":     h.t(r, "download.title"),
		"ActiveNav": "home",
		"Message":   h.t(r, "download.failed", cause),
	}), http.StatusOK)
}
