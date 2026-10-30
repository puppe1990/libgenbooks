package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"

	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func newDownloadHandler(t *testing.T, baseURL string) *DownloadHandler {
	t.Helper()
	client := libgen.New(baseURL)
	client.DownloadDir = t.TempDir()
	return NewDownloadHandler(setupTestViews(t), testSite(), appi18n.DefaultCatalog(), cais.Config{}, client)
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

func TestDownloadHandler_StreamsFileFromHandshake(t *testing.T) {
	payload := []byte("PDFDATA")
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ads.php":
			_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=KEY123">GET</a>`))
		case "/get.php":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mirror.Close)

	h := newDownloadHandler(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/download?md5=184c16eae281bf2ebea72eb6d7aa6f6d&title=Clean+Code&ext=pdf", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != string(payload) {
		t.Fatalf("body = %q, want %q", rr.Body.String(), payload)
	}
	disposition := rr.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", disposition)
	}
	if !strings.Contains(disposition, "clean-code.pdf") {
		t.Fatalf("Content-Disposition = %q, want clean-code.pdf", disposition)
	}
}

func TestDownloadHandler_MirrorFailureIs502(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(mirror.Close)

	h := newDownloadHandler(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/download?md5=184c16eae281bf2ebea72eb6d7aa6f6d", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %q", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Download failed") {
		t.Fatalf("body = %q, want download.failed copy", rr.Body.String())
	}
}
