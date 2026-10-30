package libgen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownload_ExplicitURL(t *testing.T) {
	payload := []byte("%PDF-1.4 fake book")
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/direct.epub" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client := New(srv.URL)
	dest := filepath.Join(t.TempDir(), "nested", "book.epub")
	result, err := client.Download(context.Background(), DownloadRequest{
		BookID:    42,
		DirectURL: srv.URL + "/direct.epub",
		DestPath:  dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bytes != int64(len(payload)) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, len(payload))
	}
	if result.Path != dest {
		t.Errorf("Path = %q, want %q", result.Path, dest)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("file body = %q", got)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Errorf("temporary %s.part still exists", dest)
	}
	if hits != 1 {
		t.Errorf("server hits = %d, want 1 (must not fetch the record page)", hits)
	}
}

func TestDownload_UsesRecordLinkAndCookie(t *testing.T) {
	payload := []byte("EPUBDATA")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/book/12345":
			if !strings.Contains(r.Header.Get("Cookie"), "lg_session=abc") {
				_, _ = w.Write([]byte(recordFixture))
				return
			}
			_, _ = w.Write([]byte(recordFixtureWithLinks))
		case "/get.php":
			if r.URL.Query().Get("md5") != "abc123" {
				t.Errorf("md5 = %q", r.URL.Query().Get("md5"))
			}
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	client.Cookie = "lg_session=abc"
	dest := filepath.Join(t.TempDir(), "linked.pdf")
	result, err := client.Download(context.Background(), DownloadRequest{BookID: 12345, DestPath: dest})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bytes != int64(len(payload)) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, len(payload))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("file body = %q", got)
	}
}

func TestDownload_NoLinkExplainsCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(recordFixture))
	}))
	defer srv.Close()

	client := New(srv.URL)
	_, err := client.Download(context.Background(), DownloadRequest{
		BookID:   12345,
		DestPath: filepath.Join(t.TempDir(), "book.pdf"),
	})
	if err == nil {
		t.Fatal("Download() = nil error, want error about missing link")
	}
	if !strings.Contains(err.Error(), "LIBGEN_COOKIE") {
		t.Errorf("error = %v, want hint about LIBGEN_COOKIE", err)
	}
}

func TestDownload_HTTPErrorIncludesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	client := New(srv.URL)
	dest := filepath.Join(t.TempDir(), "book.pdf")
	_, err := client.Download(context.Background(), DownloadRequest{
		DirectURL: srv.URL + "/missing.pdf",
		DestPath:  dest,
	})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want HTTP 404 in the message", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("destination %s created despite the failed download", dest)
	}
}

func TestDownload_MD5Handshake(t *testing.T) {
	payload := []byte("PDFDATA")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ads.php":
			_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=KEY123">GET</a>`))
		case "/get.php":
			if r.URL.Query().Get("key") != "KEY123" {
				t.Errorf("key = %q", r.URL.Query().Get("key"))
			}
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := New(srv.URL)
	dest := filepath.Join(t.TempDir(), "book.pdf")
	result, err := client.Download(context.Background(), DownloadRequest{
		MD5:      "184c16eae281bf2ebea72eb6d7aa6f6d",
		DestPath: dest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bytes != int64(len(payload)) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, len(payload))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("file body = %q", got)
	}
}

func TestDownload_NoSourceIsAnError(t *testing.T) {
	client := New("https://libgen.invalid")
	_, err := client.Download(context.Background(), DownloadRequest{DestPath: filepath.Join(t.TempDir(), "x.pdf")})
	if err == nil || !strings.Contains(err.Error(), "md5") {
		t.Fatalf("err = %v, want an error naming md5/id/url", err)
	}
}

func TestDownload_DerivesFilenameFromTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	client := New(srv.URL)
	downloadDir := t.TempDir()
	client.DownloadDir = downloadDir
	result, err := client.Download(context.Background(), DownloadRequest{
		BookID:    7,
		Title:     "Clean Code: A Handbook",
		Extension: "pdf",
		DirectURL: srv.URL + "/x.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(downloadDir, "clean-code-a-handbook.pdf")
	if result.Path != want {
		t.Errorf("Path = %q, want %q", result.Path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("derived destination missing: %v", err)
	}
}
