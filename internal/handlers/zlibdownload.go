package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

// linkCooldown is how long a resolved link is reused for the same reader and
// book: a reload redirects to the same signed URL instead of spending another
// unit of the account's shared daily quota.
const linkCooldown = 10 * time.Second

// cooldownSweepSize bounds the pending-link map; entries past the cooldown
// are dropped once it grows beyond this.
const cooldownSweepSize = 256

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

	mu     sync.Mutex
	recent map[string]recentLink
}

type recentLink struct {
	link string
	at   time.Time
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
	if link, ok := h.cooldownLink(r, md5); ok {
		http.Redirect(w, r, link, http.StatusFound)
		return
	}
	books, err := h.client.Search(r.Context(), title, 1)
	if err != nil {
		h.fail(w, r, md5, err.Error())
		return
	}
	match, ok := zlibrary.PickBook(books, title, strings.TrimSpace(r.URL.Query().Get("author")), strings.TrimSpace(r.URL.Query().Get("ext")), md5)
	if !ok {
		h.fail(w, r, md5, h.t(r, "download.zlib_no_copy"))
		return
	}
	link, err := h.client.DownloadLink(r.Context(), match)
	if err != nil {
		if errors.Is(err, zlibrary.ErrQuotaExhausted) {
			h.fail(w, r, md5, h.t(r, "download.zlib_quota"))
			return
		}
		h.fail(w, r, md5, err.Error())
		return
	}
	h.rememberLink(r, md5, link)
	rememberDownloadClick(h.books, r, md5, false)
	http.Redirect(w, r, link, http.StatusFound)
}

// fail records the click as failed and renders the on-site error page, so a
// resolution that dies on Z-Library's side still shows up in the reader's
// history next to the successful ones.
func (h *ZlibDownloadHandler) fail(w http.ResponseWriter, r *http.Request, md5, cause string) {
	rememberDownloadClick(h.books, r, md5, true)
	h.writeError(w, r, cause)
}

// cooldownLink returns the freshly resolved link for this reader and book,
// when one was stored a moment ago.
func (h *ZlibDownloadHandler) cooldownLink(r *http.Request, md5 string) (string, bool) {
	key, ok := cooldownKey(r, md5)
	if !ok {
		return "", false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.recent[key]
	if !ok || time.Since(entry.at) >= linkCooldown {
		return "", false
	}
	return entry.link, true
}

func (h *ZlibDownloadHandler) rememberLink(r *http.Request, md5, link string) {
	key, ok := cooldownKey(r, md5)
	if !ok {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.recent == nil {
		h.recent = map[string]recentLink{}
	}
	if len(h.recent) > cooldownSweepSize {
		cutoff := time.Now().Add(-linkCooldown)
		for stored, entry := range h.recent {
			if entry.at.Before(cutoff) {
				delete(h.recent, stored)
			}
		}
	}
	h.recent[key] = recentLink{link: link, at: time.Now()}
}

func cooldownKey(r *http.Request, md5 string) (string, bool) {
	userID, ok := session.UserID(r)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d:%s", userID, md5), true
}

func (h *ZlibDownloadHandler) writeError(w http.ResponseWriter, r *http.Request, cause string) {
	writeView(w, r, h.views, h.cfg, "app", "download", amarraData(r, h.site, map[string]any{
		"Title":     h.t(r, "download.title"),
		"ActiveNav": "home",
		"Message":   h.t(r, "download.failed", cause),
	}), http.StatusOK)
}
