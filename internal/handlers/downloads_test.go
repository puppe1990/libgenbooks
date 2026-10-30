package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

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
	if !strings.Contains(body, `href="/downloads?failed=1"`) {
		t.Fatalf("empty state missing failed filter: %s", body)
	}
}

func TestDownloadsHandler_ListsRecordedBook(t *testing.T) {
	h, books := newDownloadsHandler(t)
	if err := books.RecordDownload(catalog.DownloadRecord{
		UserID:       1,
		MD5:          "184c16eae281bf2ebea72eb6d7aa6f6d",
		Title:        "Clean Code",
		Author:       "Robert C. Martin",
		Year:         2008,
		Extension:    "pdf",
		DownloadedAt: time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/downloads", nil), 1)
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
		UserID:       1,
		MD5:          "a219d4fcad87c1cd0ea0a289c50f4883",
		Title:        "Crisis Engineering",
		Extension:    "epub",
		Failed:       true,
		DownloadedAt: time.Date(2026, 10, 30, 13, 30, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/downloads", nil), 1)
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

func TestDownloadsHandler_FiltersFailed(t *testing.T) {
	h, books := newDownloadsHandler(t)
	ok := catalog.DownloadRecord{
		UserID: 1, MD5: "184c16eae281bf2ebea72eb6d7aa6f6d", Title: "Clean Code", Extension: "pdf",
		DownloadedAt: time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC),
	}
	fail := catalog.DownloadRecord{
		UserID: 1, MD5: "a219d4fcad87c1cd0ea0a289c50f4883", Title: "Crisis Engineering", Extension: "epub", Failed: true,
		DownloadedAt: time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC),
	}
	if err := books.RecordDownload(ok); err != nil {
		t.Fatal(err)
	}
	if err := books.RecordDownload(fail); err != nil {
		t.Fatal(err)
	}

	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/downloads?failed=1", nil), 1)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "Crisis Engineering") {
		t.Fatalf("failed filter missing Crisis: %s", body)
	}
	if strings.Contains(body, "Clean Code") {
		t.Fatalf("failed filter still listed a successful download: %s", body)
	}
	if !strings.Contains(body, `href="/downloads?failed=1"`) || !strings.Contains(body, `href="/downloads"`) {
		t.Fatalf("body missing filter links: %s", body)
	}
}

func TestDownloadsHandler_FailedFilterEmpty(t *testing.T) {
	h, books := newDownloadsHandler(t)
	if err := books.RecordDownload(catalog.DownloadRecord{
		UserID: 1, MD5: "184c16eae281bf2ebea72eb6d7aa6f6d", Title: "Clean Code",
	}); err != nil {
		t.Fatal(err)
	}

	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/downloads?failed=1", nil), 1)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if strings.Contains(body, "Clean Code") {
		t.Fatalf("failed filter listed a successful download: %s", body)
	}
	if !strings.Contains(body, "No failed downloads") && !strings.Contains(body, "Nenhum download falhou") {
		t.Fatalf("body missing empty failed copy: %s", body)
	}
}

func seedHistory(t *testing.T, books *catalog.Store, userID int64, count int, failed bool) {
	t.Helper()
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		if err := books.RecordDownload(catalog.DownloadRecord{
			UserID:       userID,
			MD5:          fmt.Sprintf("%032x", i+1),
			Title:        fmt.Sprintf("Book %02d", i+1),
			Failed:       failed,
			DownloadedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func getDownloads(t *testing.T, h *DownloadsHandler, target string, userID int64) string {
	t.Helper()
	req := session.WithUserID(httptest.NewRequest(http.MethodGet, target, nil), userID)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", target, rr.Code)
	}
	return rr.Body.String()
}

func postDeleteDownload(t *testing.T, h *DownloadsHandler, userID int64, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/downloads/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Delete(rr, session.WithUserID(req, userID))
	return rr
}

func TestDownloadsHandler_RowOffersDelete(t *testing.T) {
	h, books := newDownloadsHandler(t)
	if err := books.RecordDownload(catalog.DownloadRecord{
		UserID: 1, MD5: "184c16eae281bf2ebea72eb6d7aa6f6d", Title: "Clean Code",
	}); err != nil {
		t.Fatal(err)
	}

	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/downloads", nil), 1)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	for _, want := range []string{`action="/downloads/delete"`, `name="md5" value="184c16eae281bf2ebea72eb6d7aa6f6d"`, `name="csrf_token"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
	if !strings.Contains(body, "Remove") && !strings.Contains(body, "Remover") {
		t.Fatalf("body missing the delete label: %s", body)
	}
}

func TestDownloadsHandler_DeleteRemovesRow(t *testing.T) {
	h, books := newDownloadsHandler(t)
	md5 := "184c16eae281bf2ebea72eb6d7aa6f6d"
	if err := books.RecordDownload(catalog.DownloadRecord{UserID: 1, MD5: md5, Title: "Clean Code"}); err != nil {
		t.Fatal(err)
	}

	rr := postDeleteDownload(t, h, 1, url.Values{"md5": {md5}})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body = %q", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/downloads" {
		t.Fatalf("Location = %q, want /downloads", loc)
	}
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == flash.CookieName {
			found = true
		}
	}
	if !found {
		t.Fatal("delete did not flash a notice")
	}

	got, err := books.ListDownloads(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("history = %+v, want empty after delete", got)
	}
}

func TestDownloadsHandler_DeleteKeepsFailedFilter(t *testing.T) {
	h, _ := newDownloadsHandler(t)

	rr := postDeleteDownload(t, h, 1, url.Values{
		"md5":    {"a219d4fcad87c1cd0ea0a289c50f4883"},
		"failed": {"1"},
	})

	if loc := rr.Header().Get("Location"); loc != "/downloads?failed=1" {
		t.Fatalf("Location = %q, want the failed filter", loc)
	}
}

func TestDownloadsHandler_DeleteLeavesOtherUsersRow(t *testing.T) {
	h, books := newDownloadsHandler(t)
	md5 := "184c16eae281bf2ebea72eb6d7aa6f6d"
	if err := books.RecordDownload(catalog.DownloadRecord{UserID: 2, MD5: md5, Title: "Theirs"}); err != nil {
		t.Fatal(err)
	}

	if rr := postDeleteDownload(t, h, 1, url.Values{"md5": {md5}}); rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}

	theirs, err := books.ListDownloads(2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 1 || theirs[0].Title != "Theirs" {
		t.Fatalf("user 2 history = %+v, want Theirs untouched", theirs)
	}
}

func TestDownloadsHandler_DeleteRejectsBadMD5(t *testing.T) {
	h, _ := newDownloadsHandler(t)

	rr := postDeleteDownload(t, h, 1, url.Values{"md5": {"nope"}})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadsHandler_HidesOtherUsersHistory(t *testing.T) {
	h, books := newDownloadsHandler(t)
	if err := books.RecordDownload(catalog.DownloadRecord{
		UserID: 1, MD5: "184c16eae281bf2ebea72eb6d7aa6f6d", Title: "Clean Code",
	}); err != nil {
		t.Fatal(err)
	}
	if err := books.RecordDownload(catalog.DownloadRecord{
		UserID: 2, MD5: "a219d4fcad87c1cd0ea0a289c50f4883", Title: "Crisis Engineering", Failed: true,
	}); err != nil {
		t.Fatal(err)
	}

	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/downloads", nil), 1)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "Clean Code") {
		t.Fatalf("user 1 missing own download: %s", body)
	}
	if strings.Contains(body, "Crisis Engineering") {
		t.Fatalf("user 1 saw another user's download: %s", body)
	}
}

func TestDownloadsHandler_PaginatesTenPerPage(t *testing.T) {
	h, books := newDownloadsHandler(t)
	seedHistory(t, books, 1, 12, false)

	first := getDownloads(t, h, "/downloads", 1)
	for _, want := range []string{"Book 12", "Book 03", `href="/downloads?page=2"`} {
		if !strings.Contains(first, want) {
			t.Fatalf("page 1 missing %q", want)
		}
	}
	if strings.Contains(first, "Book 02") || strings.Contains(first, "Book 01") {
		t.Fatalf("page 1 leaked rows from page 2")
	}
	if !strings.Contains(first, "Page 1 of 2") && !strings.Contains(first, "Página 1 de 2") {
		t.Fatalf("page 1 missing the page counter: %s", first)
	}

	second := getDownloads(t, h, "/downloads?page=2", 1)
	for _, want := range []string{"Book 02", "Book 01", `href="/downloads?page=1"`} {
		if !strings.Contains(second, want) {
			t.Fatalf("page 2 missing %q", want)
		}
	}
	if strings.Contains(second, "Book 12") {
		t.Fatalf("page 2 leaked rows from page 1")
	}
}

func TestDownloadsHandler_PageBeyondRangeShowsLastPage(t *testing.T) {
	h, books := newDownloadsHandler(t)
	seedHistory(t, books, 1, 12, false)

	body := getDownloads(t, h, "/downloads?page=99", 1)

	if !strings.Contains(body, "Book 01") || !strings.Contains(body, "Book 02") {
		t.Fatalf("page 99 did not fall back to the last page: %s", body)
	}
	if strings.Contains(body, "Book 12") {
		t.Fatalf("page 99 still showed page 1 rows")
	}
}

func TestDownloadsHandler_PaginationCarriesFailedFilter(t *testing.T) {
	h, books := newDownloadsHandler(t)
	seedHistory(t, books, 1, 12, true)

	body := getDownloads(t, h, "/downloads?failed=1", 1)

	if !strings.Contains(body, `href="/downloads?page=2&amp;failed=1"`) && !strings.Contains(body, `href="/downloads?page=2&failed=1"`) {
		t.Fatalf("next link dropped the failed filter: %s", body)
	}
}

func TestDownloadsHandler_DeleteKeepsCurrentPage(t *testing.T) {
	h, books := newDownloadsHandler(t)
	seedHistory(t, books, 1, 12, false)

	rr := postDeleteDownload(t, h, 1, url.Values{
		"md5":  {"00000000000000000000000000000001"},
		"page": {"2"},
	})

	if loc := rr.Header().Get("Location"); loc != "/downloads?page=2" {
		t.Fatalf("Location = %q, want the current page", loc)
	}
}

func TestDownloadsHandler_PaginationHiddenUnderTenRows(t *testing.T) {
	h, books := newDownloadsHandler(t)
	seedHistory(t, books, 1, 10, false)

	body := getDownloads(t, h, "/downloads", 1)

	if strings.Contains(body, "/downloads?page=") {
		t.Fatalf("single page rendered pagination links")
	}
	if !strings.Contains(body, "Book 10") || !strings.Contains(body, "Book 01") {
		t.Fatalf("single page missing rows")
	}
}
