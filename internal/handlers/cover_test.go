package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func newCoverHandler(t *testing.T, baseURL string) *CoverHandler {
	t.Helper()
	client := libgen.New(baseURL)
	client.DownloadDir = t.TempDir()
	return NewCoverHandler(client)
}

func TestCoverHandler_InvalidMD5Is400(t *testing.T) {
	h := newCoverHandler(t, "https://libgen.invalid")
	req := httptest.NewRequest(http.MethodGet, "/cover?md5=not-a-hash", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestCoverHandler_StreamsImage(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ads.php":
			_, _ = w.Write([]byte(`<img src="/assets/img/logo.png"><img src="/fictioncovers/4920000/184c16eae281bf2ebea72eb6d7aa6f6d.jpg">`))
		case strings.HasPrefix(r.URL.Path, "/fictioncovers/"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(jpeg)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mirror.Close)

	h := newCoverHandler(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/cover?md5=184c16eae281bf2ebea72eb6d7aa6f6d", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "image/jpeg") {
		t.Fatalf("Content-Type = %q, want image/jpeg", ct)
	}
	if rr.Body.String() != string(jpeg) {
		t.Fatalf("body = %q, want jpeg bytes", rr.Body.String())
	}
}

func TestCoverHandler_MissingCoverIsPlaceholder(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(mirror.Close)

	h := newCoverHandler(t, mirror.URL)
	req := httptest.NewRequest(http.MethodGet, "/cover?md5=184c16eae281bf2ebea72eb6d7aa6f6d", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 placeholder; body = %q", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("Content-Type = %q, want image/svg+xml", ct)
	}
	if !strings.Contains(rr.Body.String(), "<svg") {
		t.Fatalf("body = %q, want an svg placeholder", rr.Body.String())
	}
}
