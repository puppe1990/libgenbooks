package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/fakedata"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

const zlibMD5 = "b74948a3f81ea84a2a2f0b8a02735344"

func newZlibHandler(t *testing.T, client *zlibrary.Client) (*ZlibDownloadHandler, *catalog.Store) {
	t.Helper()
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })
	return NewZlibDownloadHandler(setupTestViews(t), testSite(), appi18n.DefaultCatalog(), cais.Config{}, books, client), books
}

func TestZlibDownload_DisabledWithoutAClient(t *testing.T) {
	h, _ := newZlibHandler(t, nil)
	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/download/zlib?md5="+zlibMD5+"&title=x", nil), 1)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want the on-site status page", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Z-Library") {
		t.Fatalf("body = %q, want the disabled notice", rr.Body.String())
	}
}

func TestZlibDownload_RedirectsToTheSignedFile(t *testing.T) {
	fakedata.Seed(42)
	title := fakedata.Title()
	bookHash := "abc123"
	link := fakedata.URL()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/eapi/book/search":
			payload, _ := json.Marshal(map[string]any{"success": 1, "books": []map[string]any{{
				"id": 42, "hash": bookHash, "title": title, "extension": "epub", "md5": zlibMD5,
			}}})
			_, _ = w.Write(payload)
		case "/eapi/user/login":
			_, _ = w.Write([]byte(`{"success":1,"user":{"id":7,"remix_userkey":"KEY"}}`))
		case "/eapi/book/42/" + bookHash + "/file":
			payload, _ := json.Marshal(map[string]any{"success": 1, "file": map[string]string{"downloadLink": link}})
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := zlibrary.New(srv.URL)
	client.Email = fakedata.Email()
	client.Password = fakedata.Sentence()
	h, books := newZlibHandler(t, client)

	req := session.WithUserID(httptest.NewRequest(http.MethodGet,
		"/download/zlib?md5="+zlibMD5+"&title="+url.QueryEscape(title)+"&ext=epub", nil), 1)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body = %q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != link {
		t.Fatalf("Location = %q, want %q", got, link)
	}
	history, err := books.ListDownloads(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Failed || history[0].MD5 != zlibMD5 {
		t.Fatalf("history = %+v, want the click recorded as successful", history)
	}
}

func TestZlibDownload_NoMatchingCopyStaysOnSite(t *testing.T) {
	fakedata.Seed(42)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := json.Marshal(map[string]any{"success": 1, "books": []map[string]any{{
			"id": 9, "hash": "zzz999", "title": "An Unrelated Record", "extension": "epub",
		}}})
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	client := zlibrary.New(srv.URL)
	client.Email = fakedata.Email()
	client.Password = fakedata.Sentence()
	h, _ := newZlibHandler(t, client)

	query := url.Values{"md5": {zlibMD5}, "title": {"The Way of Excellence"}, "ext": {"epub"}}
	req := session.WithUserID(httptest.NewRequest(http.MethodGet, "/download/zlib?"+query.Encode(), nil), 1)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Z-Library") {
		t.Fatalf("body = %q, want the no-copy notice", rr.Body.String())
	}
}
