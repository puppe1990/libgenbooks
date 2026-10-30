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
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ads.php":
			_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=KEY123">GET</a>`))
		case "/get.php":
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
	if strings.Contains(rr.Body.String(), "PDFDATA") {
		t.Fatal("app response contained the file body; the browser must fetch get.php")
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

func TestDownloadHandler_UnsyncedCDNStaysOnSite(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<h1>Error</h1><p>File not found. The repositories may not be synchronized, try downloading later`))
	}))
	t.Cleanup(cdn.Close)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ads.php":
			_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=KEY123">GET</a>`))
		case "/get.php":
			http.Redirect(w, r, cdn.URL+"/get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=KEY123", http.StatusTemporaryRedirect)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mirror.Close)

	h := newDownloadHandler(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/download?md5=184c16eae281bf2ebea72eb6d7aa6f6d&title=Crisis+Engineering", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 so the tab stays on our origin; Location = %q body = %q", rr.Code, rr.Header().Get("Location"), rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "" {
		t.Fatalf("Location = %q, want empty (do not send the browser to the dead CDN)", loc)
	}
	body := rr.Body.String()
	if strings.Contains(body, "booksdl") || strings.Contains(strings.ToLower(body), "cdn5") {
		t.Fatalf("body leaked a CDN URL: %s", body)
	}
	if !strings.Contains(body, "/ads.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d") {
		t.Fatalf("body missing ads.php fallback link: %s", body)
	}
	if !strings.Contains(body, "file mirror") && !strings.Contains(body, "espelho de arquivos") {
		t.Fatalf("body missing the unsynced explanation: %s", body)
	}
	got, err := h.books.ListDownloads(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Crisis Engineering" {
		t.Fatalf("history = %+v, want the click recorded even when the CDN is empty", got)
	}
}
