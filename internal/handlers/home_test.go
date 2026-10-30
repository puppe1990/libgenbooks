package handlers

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func newHomeHandler(t *testing.T) *HomeHandler {
	t.Helper()
	return newHomeHandlerWith(t, "https://libgen.invalid")
}

func newHomeHandlerWith(t *testing.T, baseURL string) *HomeHandler {
	t.Helper()
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })
	client := libgen.New(baseURL)
	client.DownloadDir = t.TempDir()
	return NewHomeHandler(setupTestViews(t), testSite(), appi18n.DefaultCatalog(), cais.Config{}, books, client)
}

func TestHomeHandler_Returns200(t *testing.T) {
	h := newHomeHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHomeHandler_RendersHTML(t *testing.T) {
	h := newHomeHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `id="amarra-main"`) {
		t.Errorf("body missing #amarra-main, got: %s", body)
	}
	if !strings.Contains(body, `name="q"`) {
		t.Errorf("body missing search field name=q, got: %s", body)
	}
	if !strings.Contains(body, "Search books") && !strings.Contains(body, "Buscar livros") {
		t.Errorf("body missing search heading, got: %s", body)
	}
}

func TestHomeHandler_ContentType(t *testing.T) {
	h := newHomeHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

// #208: a visitor without a session must not be offered app chrome that bounces
// (Dashboard) or signs out a session that does not exist.
func TestHomeHandler_anonymousHidesAuthChrome(t *testing.T) {
	h := newHomeHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if strings.Contains(body, `action="/logout"`) {
		t.Errorf("anonymous page must not offer sign-out (#208), got: %s", body)
	}
	if strings.Contains(body, `href="/dashboard"`) {
		t.Errorf("anonymous page must not link to /dashboard (#208), got: %s", body)
	}
}

func TestHomeHandler_BlankQueryDoesNotListResults(t *testing.T) {
	h := newHomeHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if strings.Contains(rr.Body.String(), "No books matched") {
		t.Fatal("empty query must not render the empty-results state")
	}
}

func TestHomeHandler_LiveQueryRendersMirrorHits(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "libgen", "testdata", "search.html"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(mirror.Close)

	h := newHomeHandlerWith(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/?q=harry+potter", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := html.UnescapeString(rr.Body.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(body, "Harry Potter à l'école des sorciers") {
		t.Fatalf("missing live title, got: %s", body)
	}
	if !strings.Contains(body, "184c16eae281bf2ebea72eb6d7aa6f6d") {
		t.Fatalf("missing md5, got: %s", body)
	}
	if !strings.Contains(body, "/download?md5=184c16eae281bf2ebea72eb6d7aa6f6d") {
		t.Fatalf("missing download link, got: %s", body)
	}
}

func TestHomeHandler_MirrorDownFallsBackToCatalog(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(mirror.Close)

	h := newHomeHandlerWith(t, mirror.URL)
	if err := h.books.UpsertBook(catalog.Book{
		ID: 42, Title: "Clean Code", Author: "Robert C. Martin",
		Year: 2008, Extension: "epub", MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Searchable: 1,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/?q=clean+code", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "Clean Code") {
		t.Fatalf("missing catalog title, got: %s", body)
	}
	if !strings.Contains(body, "local catalog") {
		t.Fatalf("missing catalog source, got: %s", body)
	}
}

func TestHomeHandler_UnknownQueryShowsEmptyState(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<table><tbody></tbody></table>`))
	}))
	t.Cleanup(mirror.Close)

	h := newHomeHandlerWith(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/?q=xyzzy-unknown", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !strings.Contains(rr.Body.String(), "No books matched") {
		t.Fatalf("missing empty state, got: %s", rr.Body.String())
	}
}
