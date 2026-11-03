package libgen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseSearch_RealMirrorPage(t *testing.T) {
	body, err := os.ReadFile("testdata/search.html")
	if err != nil {
		t.Fatal(err)
	}
	entries := ParseSearch(body, "https://libgen.li")
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	first := entries[0]
	if first.MD5 != "184c16eae281bf2ebea72eb6d7aa6f6d" {
		t.Errorf("MD5 = %q", first.MD5)
	}
	if first.ID != 100406413 {
		t.Errorf("ID = %d", first.ID)
	}
	if first.Title != "Harry Potter à l'école des sorciers 1" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.Author != "J. K. Rowling - Harry Potter- 1" {
		t.Errorf("Author = %q", first.Author)
	}
	if first.Publisher != "Bloomsbury" || first.Language != "French" {
		t.Errorf("publisher/language = %q/%q", first.Publisher, first.Language)
	}
	if first.Year != 0 {
		t.Errorf("Year = %d, want 0 for an empty year cell", first.Year)
	}
	if first.Pages != "29" || first.Extension != "epub" {
		t.Errorf("pages/extension = %q/%q", first.Pages, first.Extension)
	}
	if first.Filesize != 342000 || first.FilesizeText != "342 kB" {
		t.Errorf("filesize = %d (%q)", first.Filesize, first.FilesizeText)
	}
}

func TestParseSearch_NoResults(t *testing.T) {
	if entries := ParseSearch([]byte(`<html><body>nothing</body></html>`), "https://libgen.li"); len(entries) != 0 {
		t.Fatalf("entries = %+v, want none", entries)
	}
}

func TestParseFilesize(t *testing.T) {
	cases := map[string]int64{
		"342 kB":   342000,
		"19.8 MB":  19800000,
		"1.2 GB":   1200000000,
		"1 024 kB": 1024000,
		"":         0,
		"n/a":      0,
	}
	for input, want := range cases {
		if got := parseFilesize(input); got != want {
			t.Errorf("parseFilesize(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestSearch_HitsTheMirrorSearchPage(t *testing.T) {
	fixture, err := os.ReadFile("testdata/search.html")
	if err != nil {
		t.Fatal(err)
	}
	var seenQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.php" {
			http.NotFound(w, r)
			return
		}
		seenQuery = r.URL.RawQuery
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	entries, err := New(srv.URL).Search(context.Background(), "harry potter", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	for _, want := range []string{"req=harry+potter", "curtab=f", "res=2"} {
		if !strings.Contains(seenQuery, want) {
			t.Errorf("query %q missing %q", seenQuery, want)
		}
	}
}

func TestSearch_OrdersByNewestYear(t *testing.T) {
	var seenQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`<table><tbody></tbody></table>`))
	}))
	t.Cleanup(srv.Close)

	if _, err := New(srv.URL).Search(context.Background(), "clean code", 10); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"order=year", "ordermode=desc"} {
		if !strings.Contains(seenQuery, want) {
			t.Fatalf("query %q missing %q", seenQuery, want)
		}
	}
}

func TestSearch_FallsBackToNextMirror(t *testing.T) {
	fixture, err := os.ReadFile("testdata/search.html")
	if err != nil {
		t.Fatal(err)
	}
	var primaryHits, mirrorHits int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	t.Cleanup(primary.Close)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mirrorHits++
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(mirror.Close)

	client := New(primary.URL)
	client.DownloadMirrors = []string{mirror.URL}
	entries, err := client.Search(context.Background(), "harry potter", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 from the fallback mirror", len(entries))
	}
	if primaryHits == 0 || mirrorHits == 0 {
		t.Fatalf("hits = %d/%d, want the primary tried before the fallback", primaryHits, mirrorHits)
	}
}

func TestSearch_AllMirrorsDownNamesEveryHost(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	t.Cleanup(primary.Close)

	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	t.Cleanup(mirror.Close)

	client := New(primary.URL)
	client.DownloadMirrors = []string{mirror.URL}
	_, err := client.Search(context.Background(), "harry potter", 2)
	if err == nil {
		t.Fatal("Search() = nil error, want the joined mirror failures")
	}
	for _, host := range []string{primary.URL, mirror.URL} {
		if !strings.Contains(err.Error(), host) {
			t.Errorf("error %q missing host %s", err, host)
		}
	}
}
