package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/puppe1990/libgenbooks/internal/catalog"
)

const pendingMD5 = "54479c4878bd60adc18dc79962f4fae3"

func writePendingFile(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pending.txt")
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPendingFromFile_ParsesMD5AndFilename(t *testing.T) {
	path := writePendingFile(t, "# comentário\n\n"+pendingMD5+"|mind-over-grind.epub\n"+strings.Repeat("b", 32)+"\n")
	books, err := pendingFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("books = %+v, want 2", books)
	}
	if books[0].md5 != pendingMD5 || books[0].filename != "mind-over-grind.epub" || books[0].extension != "epub" {
		t.Fatalf("books[0] = %+v", books[0])
	}
	if books[1].md5 != strings.Repeat("b", 32) || books[1].filename != strings.Repeat("b", 32) {
		t.Fatalf("books[1] = %+v, want the md5 as the fallback file name", books[1])
	}
}

func TestPendingFromFile_RejectsBadLines(t *testing.T) {
	path := writePendingFile(t, "not-an-md5|book.epub\n")
	if _, err := pendingFromFile(path); err == nil || !strings.Contains(err.Error(), "not-an-md5") {
		t.Fatalf("err = %v, want the invalid line named", err)
	}
}

func TestDestFilename(t *testing.T) {
	cases := []struct {
		title, extension, md5, want string
	}{
		{"Learning Domain-Driven Design: Aligning Software Architecture", "epub", pendingMD5, "learning-domain-driven-design-aligning-software-architecture.epub"},
		{"", "epub", pendingMD5, pendingMD5 + ".epub"},
		{"Open to Work", "", pendingMD5, "open-to-work"},
		{"  ", ".PDF", pendingMD5, pendingMD5 + ".pdf"},
	}
	for _, c := range cases {
		if got := destFilename(c.title, c.extension, c.md5); got != c.want {
			t.Errorf("destFilename(%q, %q) = %q, want %q", c.title, c.extension, got, c.want)
		}
	}
}

func TestCollectPending_ScopesUserAndFallsBackToFile(t *testing.T) {
	store, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	seed := func(userID int64, md5 string, failed bool) {
		t.Helper()
		if err := store.RecordDownload(catalog.DownloadRecord{
			UserID: userID, MD5: md5, Title: "The Way of Excellence", Extension: "epub", Failed: failed,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seed(1, pendingMD5, true)
	seed(1, strings.Repeat("b", 32), false)
	seed(2, strings.Repeat("c", 32), true)

	all, err := collectPending(store, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %+v, want both users' failed rows", all)
	}

	scoped, err := collectPending(store, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].userID != 1 || scoped[0].md5 != pendingMD5 {
		t.Fatalf("scoped = %+v, want only user 1's failed row", scoped)
	}
	if scoped[0].filename != "the-way-of-excellence.epub" {
		t.Fatalf("filename = %q", scoped[0].filename)
	}

	fromFile, err := collectPending(store, writePendingFile(t, pendingMD5+"\n"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(fromFile) != 1 || fromFile[0].userID != 0 {
		t.Fatalf("fromFile = %+v, want the file list with no history row", fromFile)
	}
}

func TestMarkRetrySuccess_ClearsTheFailedRow(t *testing.T) {
	store, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.RecordDownload(catalog.DownloadRecord{UserID: 1, MD5: pendingMD5, Title: "Mind Over Grind", Extension: "epub", Failed: true}); err != nil {
		t.Fatal(err)
	}
	markRetrySuccess(store, pendingBook{userID: 1, md5: pendingMD5, title: "Mind Over Grind", extension: "epub"})

	failed, err := store.FailedDownloads(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Fatalf("failed = %+v, want the row cleared after the retry succeeded", failed)
	}
	all, err := store.ListDownloads(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Failed || all[0].Title != "Mind Over Grind" {
		t.Fatalf("history = %+v, want one successful Mind Over Grind row", all)
	}
}
