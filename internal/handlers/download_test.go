package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func newDownloadHandler(t *testing.T, baseURL string) *DownloadHandler {
	t.Helper()
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })
	client := libgen.New(baseURL)
	client.DownloadDir = t.TempDir()
	return NewDownloadHandler(setupTestViews(t), testSite(), appi18n.DefaultCatalog(), cais.Config{}, books, client)
}

func TestDownloadHandler_MissingMD5Is400(t *testing.T) {
	h := newDownloadHandler(t, "https://libgen.invalid")
	req := httptest.NewRequest(http.MethodGet, "/download", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "md5") {
		t.Fatalf("body = %q, want an md5 error", rr.Body.String())
	}
}

func TestDownloadHandler_InvalidMD5Is400(t *testing.T) {
	h := newDownloadHandler(t, "https://libgen.invalid")
	req := httptest.NewRequest(http.MethodGet, "/download?md5=not-a-hash", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadHandler_RedirectsToHandshakeLink(t *testing.T) {
	var getHits int
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ads.php":
			_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=KEY123">GET</a>`))
		case "/get.php":
			getHits++
			_, _ = w.Write([]byte("PDFDATA"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mirror.Close)

	h := newDownloadHandler(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/download?md5=184c16eae281bf2ebea72eb6d7aa6f6d&title=Clean+Code&ext=pdf", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body = %q", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if !strings.Contains(location, "/get.php?") || !strings.Contains(location, "key=KEY123") {
		t.Fatalf("Location = %q, want the ads.php get.php link", location)
	}
	if getHits != 0 {
		t.Fatalf("get.php hits = %d, want 0 (browser fetches the file, not the app)", getHits)
	}
	got, err := h.books.ListDownloads(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Clean Code" || got[0].Extension != "pdf" {
		t.Fatalf("history = %+v, want Clean Code pdf recorded on click", got)
	}
}

func TestDownloadHandler_HandshakeFailureRedirectsToAdsPage(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(mirror.Close)

	h := newDownloadHandler(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/download?md5=184c16eae281bf2ebea72eb6d7aa6f6d", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body = %q", rr.Code, rr.Body.String())
	}
	want := mirror.URL + "/ads.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d"
	if got := rr.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want the ads.php page so the browser retries with a residential IP", got)
	}
}
