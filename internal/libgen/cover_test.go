package libgen

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const harryMD5 = "184c16eae281bf2ebea72eb6d7aa6f6d"

// tinyJPEG is a valid empty JPEG (SOI + EOI). Sniffing only needs the magic.
var tinyJPEG = []byte{0xFF, 0xD8, 0xFF, 0xD9}

func adsCoverHTML(md5 string) []byte {
	return []byte(`<html><body>
<img src="/assets/img/logo.png" alt="libgen">
<img src="/fictioncovers/4920000/` + md5 + `.jpg" alt="cover">
</body></html>`)
}

func TestResolveCover_FromAdsPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ads.php" || r.URL.Query().Get("md5") != harryMD5 {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(adsCoverHTML(harryMD5))
	}))
	t.Cleanup(srv.Close)

	cover, err := New(srv.URL).ResolveCover(context.Background(), harryMD5)
	if err != nil {
		t.Fatal(err)
	}
	want := srv.URL + "/fictioncovers/4920000/" + harryMD5 + ".jpg"
	if cover != want {
		t.Fatalf("cover = %q, want %q", cover, want)
	}
}

func TestResolveCover_RejectsBadMD5(t *testing.T) {
	if _, err := New("https://libgen.li").ResolveCover(context.Background(), "nope"); err == nil {
		t.Fatal("ResolveCover(nope) = nil error, want error")
	}
}

func TestResolveCover_RejectsOffHostImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<img src="https://evil.example/ssrf.jpg">`))
	}))
	t.Cleanup(srv.Close)

	if _, err := New(srv.URL).ResolveCover(context.Background(), harryMD5); err == nil {
		t.Fatal("off-host cover must be rejected")
	}
}

func TestFetchCover_WritesJPEGCache(t *testing.T) {
	srv := newCoverMirror(t, tinyJPEG)
	client := New(srv.URL)
	client.DownloadDir = t.TempDir()

	path, contentType, err := client.FetchCover(context.Background(), harryMD5)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/jpeg" {
		t.Fatalf("contentType = %q, want image/jpeg", contentType)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, tinyJPEG) {
		t.Fatalf("cached %d bytes, want %d", len(got), len(tinyJPEG))
	}
}

func TestFetchCover_UsesDiskCache(t *testing.T) {
	var adsHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ads.php":
			adsHits++
			_, _ = w.Write(adsCoverHTML(harryMD5))
		case strings.HasPrefix(r.URL.Path, "/fictioncovers/"):
			_, _ = w.Write(tinyJPEG)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL)
	client.DownloadDir = t.TempDir()
	if _, _, err := client.FetchCover(context.Background(), harryMD5); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.FetchCover(context.Background(), harryMD5); err != nil {
		t.Fatal(err)
	}
	if adsHits != 1 {
		t.Fatalf("ads.php hits = %d, want 1 (second call is cache)", adsHits)
	}
}

func TestFetchCover_RejectsOversizedBody(t *testing.T) {
	tooBig := bytes.Repeat([]byte{0xFF, 0xD8, 0xFF, 0xD9}, (2<<20)/4+1)
	srv := newCoverMirror(t, tooBig)
	client := New(srv.URL)
	client.DownloadDir = t.TempDir()

	if _, _, err := client.FetchCover(context.Background(), harryMD5); err == nil {
		t.Fatal("oversized cover must be rejected")
	}
}

func newCoverMirror(t *testing.T, jpeg []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ads.php":
			_, _ = w.Write(adsCoverHTML(harryMD5))
		case strings.HasPrefix(r.URL.Path, "/fictioncovers/"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(jpeg)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
