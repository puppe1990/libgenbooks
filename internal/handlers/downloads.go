package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/libgenbooks/internal/catalog"
)

const historyPageSize = 10

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
	failedOnly := failedDownloadsFilter(r)
	history := h.history(r, failedOnly)
	writeView(w, r, h.views, h.cfg, "app", "downloads", amarraData(r, h.site, map[string]any{
		"Title":      h.t(r, "downloads.title"),
		"ActiveNav":  "downloads",
		"Books":      history.Books,
		"FailedOnly": failedOnly,
		"Page":       history.Number,
		"TotalPages": history.Total,
		"PrevPage":   history.Number - 1,
		"NextPage":   history.Number + 1,
	}), 0)
}

// Delete removes one row from the signed-in reader's history.
func (h *DownloadsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := session.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	md5 := strings.ToLower(strings.TrimSpace(r.FormValue("md5")))
	if !catalog.IsMD5(md5) {
		http.Error(w, h.t(r, "downloads.delete_md5"), http.StatusBadRequest)
		return
	}
	if err := h.books.DeleteDownload(userID, md5); err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}
	flash.Set(w, "notice", h.t(r, "downloads.deleted"), h.cfg.CookieSecure())
	http.Redirect(w, r, historyPath(r.FormValue("failed"), r.FormValue("page")), http.StatusSeeOther)
}

func failedDownloadsFilter(r *http.Request) bool {
	return strings.TrimSpace(r.URL.Query().Get("failed")) == "1"
}

func historyPath(failed, page string) string {
	number, _ := strconv.Atoi(strings.TrimSpace(page))
	params := url.Values{}
	if number > 1 {
		params.Set("page", strconv.Itoa(number))
	}
	if strings.TrimSpace(failed) == "1" {
		params.Set("failed", "1")
	}
	if len(params) == 0 {
		return "/downloads"
	}
	return "/downloads?" + params.Encode()
}

func (h *DownloadsHandler) history(r *http.Request, failedOnly bool) downloadHistory {
	userID, ok := session.UserID(r)
	if h.books == nil || !ok {
		return downloadHistory{Number: 1}
	}
	total, err := h.books.CountDownloads(userID, failedOnly)
	if err != nil {
		return downloadHistory{Number: 1}
	}
	pages := pageCount(total, historyPageSize)
	number := clampPage(requestedPage(r), pages)
	records, err := h.books.ListDownloadsPage(userID, historyPageSize, (number-1)*historyPageSize, failedOnly)
	if err != nil {
		return downloadHistory{Number: number, Total: pages}
	}
	return downloadHistory{Books: historyItems(records), Number: number, Total: pages}
}

func pageCount(total, size int) int {
	if total <= 0 {
		return 0
	}
	return (total + size - 1) / size
}

func clampPage(page, pages int) int {
	if pages < 1 || page < 1 {
		return 1
	}
	if page > pages {
		return pages
	}
	return page
}

func requestedPage(r *http.Request) int {
	page, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if err != nil {
		return 1
	}
	return page
}

func historyItems(records []catalog.DownloadRecord) []downloadHistoryItem {
	items := make([]downloadHistoryItem, 0, len(records))
	for _, rec := range records {
		items = append(items, downloadHistoryItem{
			MD5:       rec.MD5,
			Title:     rec.Title,
			Author:    rec.Author,
			Year:      rec.Year,
			Extension: rec.Extension,
			When:      rec.DownloadedAt.UTC().Format("02/01/2006 15:04"),
			Failed:    rec.Failed,
		})
	}
	return items
}

type downloadHistory struct {
	Books  []downloadHistoryItem
	Number int
	Total  int
}

type downloadHistoryItem struct {
	MD5       string
	Title     string
	Author    string
	Year      int
	Extension string
	When      string
	Failed    bool
}
