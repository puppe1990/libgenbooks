package libgen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetLinkUnsynced_DetectsHTML500(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<h1>Error</h1><p>File not found. The repositories may not be synchronized, try downloading later`))
	}))
	t.Cleanup(cdn.Close)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cdn.URL+"/get.php?md5="+harryMD5+"&key=K", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(mirror.Close)

	client := New(mirror.URL)
	getURL := mirror.URL + "/get.php?md5=" + harryMD5 + "&key=K"
	if !client.GetLinkUnsynced(context.Background(), getURL) {
		t.Fatal("GetLinkUnsynced = false, want true for the booksdl unsynced HTML")
	}
}

func TestGetLinkUnsynced_FalseForOctetStream(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="book.epub"`)
		_, _ = w.Write([]byte("EPUBDATA"))
	}))
	t.Cleanup(cdn.Close)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cdn.URL+"/get.php?md5="+harryMD5+"&key=K", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(mirror.Close)

	client := New(mirror.URL)
	getURL := mirror.URL + "/get.php?md5=" + harryMD5 + "&key=K"
	if client.GetLinkUnsynced(context.Background(), getURL) {
		t.Fatal("GetLinkUnsynced = true, want false when the CDN serves the file")
	}
}

func TestGetLinkUnsynced_FalseOnTimeout(t *testing.T) {
	old := peekTimeout
	peekTimeout = 80 * time.Millisecond
	t.Cleanup(func() { peekTimeout = old })

	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		_, _ = w.Write([]byte("slow"))
	}))
	t.Cleanup(cdn.Close)

	client := New(cdn.URL)
	if client.GetLinkUnsynced(context.Background(), cdn.URL+"/get.php") {
		t.Fatal("GetLinkUnsynced = true, want false so a blocked datacenter IP still 302s the browser")
	}
}

func TestGetLinkUnsynced_FalseForEmptyURL(t *testing.T) {
	if New("https://libgen.li").GetLinkUnsynced(context.Background(), "  ") {
		t.Fatal("empty get URL must not look unsynced")
	}
}

func TestUnsyncedCDNResponse(t *testing.T) {
	tests := []struct {
		status int
		ctype  string
		want   bool
	}{
		{500, "text/html; charset=UTF-8", true},
		{404, "text/html", true},
		{500, "", true},
		{200, "application/octet-stream", false},
		{206, "application/pdf", false},
		{403, "text/html", false},
		{502, "text/html", false},
	}
	for _, tc := range tests {
		got := unsyncedCDNResponse(tc.status, tc.ctype)
		if got != tc.want {
			t.Errorf("unsyncedCDNResponse(%d, %q) = %v, want %v", tc.status, tc.ctype, got, tc.want)
		}
	}
}

func TestGetLinkUnsynced_HEADOnly(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("File not found. The repositories may not be synchronized"))
	}))
	t.Cleanup(srv.Close)

	if !New(srv.URL).GetLinkUnsynced(context.Background(), srv.URL+"/get.php") {
		t.Fatal("GetLinkUnsynced = false, want true")
	}
	if len(methods) != 1 || methods[0] != http.MethodHead {
		t.Fatalf("methods = %v, want a single HEAD (never GET the book)", methods)
	}
}

func TestGetLinkUnsynced_IgnoresOffTopicBody(t *testing.T) {
	// sanity: the helper is status+content-type, so a 200 HTML ads page is not unsynced
	if unsyncedCDNResponse(200, "text/html") {
		t.Fatal("200 HTML must not count as an unsynced file CDN")
	}
	if !strings.Contains("text/html; charset=UTF-8", "text/html") {
		t.Fatal("sanity")
	}
}
