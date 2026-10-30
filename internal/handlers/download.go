package handlers

import (
	"net/http"
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
	client  *libgen.Client
}

func NewDownloadHandler(views *view.Renderer, site meta.Site, messages *i18n.Catalog, cfg cais.Config, client *libgen.Client) *DownloadHandler {
	return &DownloadHandler{
		views:   views,
		site:    site,
		catalog: messages,
		cfg:     cfg,
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

func (h *DownloadHandler) writeDownloadError(w http.ResponseWriter, r *http.Request, err error) {
	writeView(w, r, h.views, h.cfg, "app", "download", amarraData(r, h.site, map[string]any{
		"Title":     h.t(r, "download.title"),
		"ActiveNav": "home",
		"Message":   h.t(r, "download.failed", err.Error()),
	}), http.StatusOK)
}

