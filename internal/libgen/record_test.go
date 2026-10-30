package libgen

import (
	"strings"
	"testing"
)

const recordFixture = `<!doctype html><html lang="en"><head><title>Concise Handbook - Library Genesis</title></head><body>
<nav><a href="/mirrors">Mirrors</a><a href="/torrents">Torrents</a></nav>
<main class="container-fluid pb-3"><div class="lg-card p-3"><div class="row">
<div class="col-md-2 text-center mb-3"><img class="img-fluid border" loading="lazy" src="https://isbncoverdb.com/images/book-cover-concise.webp" alt=""></div>
<div class="col-md-10"><h2 style="font-size:1.5rem;color:#7b0000">Concise Handbook &amp; Waste Treatment</h2>
<table class="table table-sm lg-table">
<tr><th style="width:150px">Author(s)</th><td>Saleh S. Al Arni, Mahmoud M. Elwaheidi</td></tr>
<tr><th>Publisher</th><td>CRC Press</td></tr>
<tr><th>Year</th><td>2022</td></tr>
<tr><th>Language</th><td>English</td></tr>
<tr><th>Extension</th><td>pdf</td></tr>
<tr><th>Filesize</th><td>19.8 MB</td></tr>
<tr><th>Pages</th><td>158</td></tr>
<tr><th>ISBN</th><td>9780367631307,9780367631291</td></tr>
<tr><th>Record ID</th><td>12345</td></tr>
</table>
<h5>Description</h5><p>With specialized and succinct coverage, this handbook summarizes waste treatments.</p>
</div></div></div></main>
</body></html>`

const recordFixtureWithLinks = `<!doctype html><html lang="en"><head><title>Linked - Library Genesis</title></head><body>
<main><div class="lg-card p-3"><div class="row">
<div class="col-md-10"><h2>Linked Book</h2>
<table class="table table-sm lg-table">
<tr><th>Author(s)</th><td>Someone</td></tr>
<tr><th>Record ID</th><td>777</td></tr>
</table>
<table class="table"><tr>
<td><a href="/get.php?md5=abc123&amp;key=xyz">GET</a></td>
<td><a href="https://cdn.example.org/linked.pdf">Cloudflare</a></td>
</tr></table>
</div></div></div></main>
</body></html>`

func TestParseRecord_FixturePage(t *testing.T) {
	rec, err := ParseRecord([]byte(recordFixture), "https://libgen.ad/book/12345")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != 12345 {
		t.Errorf("ID = %d, want 12345", rec.ID)
	}
	if rec.Title != "Concise Handbook & Waste Treatment" {
		t.Errorf("Title = %q", rec.Title)
	}
	if rec.Author != "Saleh S. Al Arni, Mahmoud M. Elwaheidi" {
		t.Errorf("Author = %q", rec.Author)
	}
	if rec.Publisher != "CRC Press" || rec.Year != "2022" || rec.Extension != "pdf" {
		t.Errorf("metadata = %+v", rec)
	}
	if rec.Pages != "158" || rec.Filesize != "19.8 MB" {
		t.Errorf("pages/filesize = %q/%q", rec.Pages, rec.Filesize)
	}
	if !strings.Contains(rec.ISBN, "9780367631307") {
		t.Errorf("ISBN = %q", rec.ISBN)
	}
	if !strings.Contains(rec.Description, "summarizes waste treatments") {
		t.Errorf("Description = %q", rec.Description)
	}
	if rec.CoverURL != "https://isbncoverdb.com/images/book-cover-concise.webp" {
		t.Errorf("CoverURL = %q", rec.CoverURL)
	}
	if len(rec.DownloadURLs) != 0 {
		t.Errorf("DownloadURLs = %v, want none on an anonymous page", rec.DownloadURLs)
	}
	if rec.URL != "https://libgen.ad/book/12345" {
		t.Errorf("URL = %q", rec.URL)
	}
}

func TestParseRecord_DownloadLinks(t *testing.T) {
	rec, err := ParseRecord([]byte(recordFixtureWithLinks), "https://libgen.ad/book/777")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://libgen.ad/get.php?md5=abc123&key=xyz",
		"https://cdn.example.org/linked.pdf",
	}
	if len(rec.DownloadURLs) != len(want) {
		t.Fatalf("DownloadURLs = %v, want %v", rec.DownloadURLs, want)
	}
	for i := range want {
		if rec.DownloadURLs[i] != want[i] {
			t.Errorf("DownloadURLs[%d] = %q, want %q", i, rec.DownloadURLs[i], want[i])
		}
	}
}

func TestParseRecord_RejectsGarbage(t *testing.T) {
	if _, err := ParseRecord([]byte(`<html><body>nothing here</body></html>`), "https://libgen.ad/book/1"); err == nil {
		t.Fatal("ParseRecord(garbage) = nil error, want error")
	}
}
