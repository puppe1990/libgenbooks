package libgen

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveDownload_ReadsAdsPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ads.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<table><tr><td><a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=EL9A1BQ2G4UM7ZWN"><h2>GET</h2></a></td></tr></table>`))
	}))
	defer srv.Close()

	link, err := New(srv.URL).ResolveDownload(context.Background(), "184c16eae281bf2ebea72eb6d7aa6f6d")
	if err != nil {
		t.Fatal(err)
	}
	want := srv.URL + "/get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=EL9A1BQ2G4UM7ZWN"
	if link != want {
		t.Errorf("link = %q, want %q", link, want)
	}
}

func TestResolveDownload_RejectsBadMD5(t *testing.T) {
	if _, err := New("https://libgen.li").ResolveDownload(context.Background(), "nope"); err == nil {
		t.Fatal("ResolveDownload(nope) = nil error, want error")
	}
}

func TestResolveDownload_RejectsOffHostGetLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="https://evil.example/get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=X">GET</a>`))
	}))
	t.Cleanup(srv.Close)

	if _, err := New(srv.URL).ResolveDownload(context.Background(), harryMD5); err == nil {
		t.Fatal("off-host get.php must be rejected")
	}
}

func TestResolveDownload_RetriesAdsPage(t *testing.T) {
	var adsHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adsHits++
		if adsHits == 1 {
			http.Error(w, "busy", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=RETRYKEY">GET</a>`))
	}))
	t.Cleanup(srv.Close)

	link, err := New(srv.URL).ResolveDownload(context.Background(), harryMD5)
	if err != nil {
		t.Fatal(err)
	}
	if adsHits != 2 {
		t.Fatalf("ads.php hits = %d, want 2", adsHits)
	}
	if !strings.Contains(link, "key=RETRYKEY") {
		t.Fatalf("link = %q, want the retried get.php key", link)
	}
}

func TestResolveDownload_FallsBackToNextMirror(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	t.Cleanup(primary.Close)

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=VGKEY">GET</a>`))
	}))
	t.Cleanup(fallback.Close)

	client := New(primary.URL)
	client.DownloadMirrors = []string{fallback.URL}
	link, err := client.ResolveDownload(context.Background(), harryMD5)
	if err != nil {
		t.Fatal(err)
	}
	want := fallback.URL + "/get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=VGKEY"
	if link != want {
		t.Fatalf("link = %q, want %q", link, want)
	}
}

func TestResolveDownload_RacesMirrors(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	t.Cleanup(slow.Close)

	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=FASTKEY">GET</a>`))
	}))
	t.Cleanup(fast.Close)

	client := New(slow.URL)
	client.DownloadMirrors = []string{fast.URL}
	start := time.Now()
	link, err := client.ResolveDownload(context.Background(), harryMD5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(link, "key=FASTKEY") {
		t.Fatalf("link = %q, want the fast mirror's link", link)
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("ResolveDownload took %v: mirrors must race in parallel, not wait on the slow one", elapsed)
	}
}

func TestGet_StatusErrorIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	_, err := New(srv.URL).get(context.Background(), srv.URL+"/ads.php")
	var statusErr statusError
	if !errors.As(err, &statusErr) || statusErr.status != http.StatusBadGateway {
		t.Fatalf("err = %v, want a typed statusError with status 502", err)
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Errorf("error %q must keep the human message", err)
	}
}

func TestRetryableFetch_TypedStatuses(t *testing.T) {
	retryable := []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusTooManyRequests}
	for _, status := range retryable {
		err := statusError{rawURL: "https://libgen.li/ads.php", status: status}
		if !retryableFetch(err) {
			t.Errorf("retryableFetch(status %d) = false, want true", status)
		}
	}
	if retryableFetch(statusError{rawURL: "https://libgen.li/ads.php", status: http.StatusNotFound}) {
		t.Error("404 must not be retryable")
	}
	if retryableFetch(context.Canceled) {
		t.Error("context cancellation must not be retryable")
	}
}

func TestAdsPageURL_UsesFirstFallbackMirror(t *testing.T) {
	client := New("https://libgen.li")
	client.DownloadMirrors = []string{"https://libgen.vg", "https://libgen.la"}
	got := client.AdsPageURL(harryMD5)
	want := "https://libgen.vg/ads.php?md5=" + harryMD5
	if got != want {
		t.Fatalf("AdsPageURL = %q, want %q", got, want)
	}
}
