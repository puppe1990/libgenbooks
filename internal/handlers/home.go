package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/puppe1990/libgenbooks/internal/cache"
	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

const webSearchLimit = 25

type HomeHandler struct {
	views   *view.Renderer
	site    meta.Site
	catalog *i18n.Catalog
	cfg     cais.Config
	books   *catalog.Store
	client  *libgen.Client
}

func NewHomeHandler(views *view.Renderer, site meta.Site, messages *i18n.Catalog, cfg cais.Config, books *catalog.Store, client *libgen.Client) *HomeHandler {
	return &HomeHandler{
		views:   views,
		site:    site,
		catalog: messages,
		cfg:     cfg,
		books:   books,
		client:  client,
	}
}

// t translates Go-side copy with the request locale (#211); h.catalog is the
// boot fallback for requests that skipped LocaleMiddleware (unit tests).
func (h *HomeHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *HomeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	data := map[string]any{
		"Title":     h.t(r, "home.title"),
		"ActiveNav": "home",
		"Query":     query,
		"Searched":  query != "",
	}
	if query != "" {
		books, source, hint := h.lookup(r.Context(), query)
		data["Books"] = books
		data["Source"] = h.t(r, source)
		data["Hint"] = hint
	}
	writeView(w, r, h.views, h.cfg, "app", "home", amarraData(r, h.site, data), 0)
}

func (h *HomeHandler) lookup(ctx context.Context, query string) ([]libgen.Entry, string, string) {
	entries, err := h.client.Search(ctx, query, webSearchLimit)
	if err != nil {
		return h.lookupCatalog(query, err)
	}
	if len(entries) > 0 && h.books != nil {
		cache.SaveEntries(h.books, entries)
	}
	return entries, "home.source_mirror", ""
}

func (h *HomeHandler) lookupCatalog(query string, liveErr error) ([]libgen.Entry, string, string) {
	hint := liveErr.Error()
	if h.books == nil {
		return nil, "home.source_catalog", hint
	}
	found, err := h.books.Search(query, webSearchLimit)
	if err != nil || len(found) == 0 {
		return nil, "home.source_catalog", hint
	}
	return entriesFromBooks(found), "home.source_catalog", hint
}

func entriesFromBooks(books []catalog.Book) []libgen.Entry {
	entries := make([]libgen.Entry, 0, len(books))
	for _, book := range books {
		entries = append(entries, libgen.Entry{
			ID:        book.ID,
			MD5:       book.MD5,
			Title:     book.Title,
			Author:    book.Author,
			Year:      book.Year,
			Extension: book.Extension,
			Filesize:  book.Filesize,
		})
	}
	return entries
}
