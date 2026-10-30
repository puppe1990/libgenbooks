package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

type DownloadHandler struct {
	views   *view.Renderer
	site    meta.Site
	catalog *i18n.Catalog
	cfg     cais.Config
	books   *catalog.Store
	client  *libgen.Client
}

func NewDownloadHandler(views *view.Renderer, site meta.Site, messages *i18n.Catalog, cfg cais.Config, books *catalog.Store, client *libgen.Client) *DownloadHandler {
	return &DownloadHandler{
		views:   views,
		site:    site,
		catalog: messages,
		cfg:     cfg,
		books:   books,
		client:  client,
	}
}

func (h *DownloadHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *DownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	md5 := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("md5")))
	if !catalog.IsMD5(md5) {
		http.Error(w, h.t(r, "download.missing_md5"), http.StatusBadRequest)
		return
	}
	h.rememberClick(r, md5)

	// The file CDN (booksdl.lc) blocks datacenter IPs. Resolve the signed
	// get.php URL here and send the browser there, so the download uses the
	// reader's residential IP instead of the Cleat box.
	link, err := h.client.ResolveDownload(r.Context(), md5)
	if err != nil {
		if ads := h.client.AdsPageURL(md5); ads != "" {
			http.Redirect(w, r, ads, http.StatusFound)
			return
		}
		h.writeDownloadError(w, r, err)
		return
	}
	http.Redirect(w, r, link, http.StatusFound)
}

func (h *DownloadHandler) rememberClick(r *http.Request, md5 string) {
	if h.books == nil {
		return
	}
	rec := catalog.DownloadRecord{
		MD5:       md5,
		Title:     strings.TrimSpace(r.URL.Query().Get("title")),
		Author:    strings.TrimSpace(r.URL.Query().Get("author")),
		Extension: strings.TrimSpace(r.URL.Query().Get("ext")),
	}
	if year, err := strconv.Atoi(r.URL.Query().Get("year")); err == nil {
		rec.Year = year
	}
	fillDownloadFromCatalog(h.books, &rec)
	_ = h.books.RecordDownload(rec)
}

func fillDownloadFromCatalog(books *catalog.Store, rec *catalog.DownloadRecord) {
	found, err := books.FindByMD5(rec.MD5)
	if err != nil {
		return
	}
	if rec.Title == "" {
		rec.Title = found.Title
	}
	if rec.Author == "" {
		rec.Author = found.Author
	}
	if rec.Year == 0 {
		rec.Year = found.Year
	}
	if rec.Extension == "" {
		rec.Extension = found.Extension
	}
	if rec.Filesize == 0 {
		rec.Filesize = found.Filesize
	}
}

func (h *DownloadHandler) writeDownloadError(w http.ResponseWriter, r *http.Request, err error) {
	writeView(w, r, h.views, h.cfg, "app", "download", amarraData(r, h.site, map[string]any{
		"Title":     h.t(r, "download.title"),
		"ActiveNav": "home",
		"Message":   h.t(r, "download.failed", err.Error()),
	}), http.StatusOK)
}

