package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
)

func newDownloadsHandler(t *testing.T) (*DownloadsHandler, *catalog.Store) {
	t.Helper()
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })
	h := NewDownloadsHandler(setupTestViews(t), testSite(), appi18n.DefaultCatalog(), cais.Config{}, books)
	return h, books
}

func TestDownloadsHandler_EmptyState(t *testing.T) {
	h, _ := newDownloadsHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/downloads", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "No downloads yet") && !strings.Contains(body, "Nenhum download") {
		t.Fatalf("body missing empty copy: %s", body)
	}
	if !strings.Contains(body, `href="/downloads"`) {
		t.Fatalf("nav missing /downloads, got: %s", body)
	}
}

func TestDownloadsHandler_ListsRecordedBook(t *testing.T) {
	h, books := newDownloadsHandler(t)
	if err := books.RecordDownload(catalog.DownloadRecord{
		MD5:          "184c16eae281bf2ebea72eb6d7aa6f6d",
		Title:        "Clean Code",
		Author:       "Robert C. Martin",
		Year:         2008,
		Extension:    "pdf",
		DownloadedAt: time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/downloads", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Clean Code", "Robert C. Martin", "2008", "pdf", "/cover?md5=184c16eae281bf2ebea72eb6d7aa6f6d", "/download?md5=184c16eae281bf2ebea72eb6d7aa6f6d"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
	if strings.Contains(body, "Download failed") || strings.Contains(body, "Download falhou") {
		t.Fatalf("successful row showed a failed mark: %s", body)
	}
}

func TestDownloadsHandler_MarksFailedClick(t *testing.T) {
	h, books := newDownloadsHandler(t)
	if err := books.RecordDownload(catalog.DownloadRecord{
		MD5:          "a219d4fcad87c1cd0ea0a289c50f4883",
		Title:        "Crisis Engineering",
		Extension:    "epub",
		Failed:       true,
		DownloadedAt: time.Date(2026, 10, 30, 13, 30, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/downloads", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "Crisis Engineering") {
		t.Fatalf("body missing the clicked title: %s", body)
	}
	if !strings.Contains(body, "Download failed") && !strings.Contains(body, "Download falhou") {
		t.Fatalf("body missing the failed mark: %s", body)
	}
}
