package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
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

	result, err := h.client.Download(r.Context(), libgen.DownloadRequest{
		MD5:       md5,
		Title:     r.URL.Query().Get("title"),
		Extension: r.URL.Query().Get("ext"),
	})
	if err != nil {
		http.Error(w, h.t(r, "download.failed", err.Error()), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Disposition", attachmentDisposition(filepath.Base(result.Path)))
	http.ServeFile(w, r, result.Path)
}

func attachmentDisposition(filename string) string {
	return fmt.Sprintf(`attachment; filename="%s"`, filename)
}
