package handlers

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/puppe1990/libgenbooks/internal/catalog"
)

type DownloadsHandler struct {
	views   *view.Renderer
	site    meta.Site
	catalog *i18n.Catalog
	cfg     cais.Config
	books   *catalog.Store
}

func NewDownloadsHandler(views *view.Renderer, site meta.Site, messages *i18n.Catalog, cfg cais.Config, books *catalog.Store) *DownloadsHandler {
	return &DownloadsHandler{
		views:   views,
		site:    site,
		catalog: messages,
		cfg:     cfg,
		books:   books,
	}
}

func (h *DownloadsHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *DownloadsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	items := h.history()
	writeView(w, r, h.views, h.cfg, "app", "downloads", amarraData(r, h.site, map[string]any{
		"Title":     h.t(r, "downloads.title"),
		"ActiveNav": "downloads",
		"Books":     items,
	}), 0)
}

func (h *DownloadsHandler) history() []downloadHistoryItem {
	if h.books == nil {
		return nil
	}
	records, err := h.books.ListDownloads(100)
	if err != nil {
		return nil
	}
	items := make([]downloadHistoryItem, 0, len(records))
	for _, rec := range records {
		items = append(items, downloadHistoryItem{
			MD5:       rec.MD5,
			Title:     rec.Title,
			Author:    rec.Author,
			Year:      rec.Year,
			Extension: rec.Extension,
			When:      rec.DownloadedAt.UTC().Format("02/01/2006 15:04"),
		})
	}
	return items
}

type downloadHistoryItem struct {
	MD5       string
	Title     string
	Author    string
	Year      int
	Extension string
	When      string
}
