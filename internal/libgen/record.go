package libgen

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Record is the metadata the public /book/<id> page exposes.
type Record struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Author       string   `json:"author"`
	Publisher    string   `json:"publisher"`
	Year         string   `json:"year"`
	Language     string   `json:"language"`
	Extension    string   `json:"extension"`
	Filesize     string   `json:"filesize"`
	Pages        string   `json:"pages"`
	ISBN         string   `json:"isbn"`
	Description  string   `json:"description"`
	CoverURL     string   `json:"cover_url"`
	URL          string   `json:"url"`
	DownloadURLs []string `json:"download_urls"`
}

var (
	titleTagRE  = regexp.MustCompile(`(?is)<h2[^>]*>(.*?)</h2>`)
	metaRowRE   = regexp.MustCompile(`(?is)<tr[^>]*>\s*<th[^>]*>(.*?)</th>\s*<td[^>]*>(.*?)</td>`)
	descBlockRE = regexp.MustCompile(`(?is)<h5[^>]*>\s*Description\s*</h5>\s*<p[^>]*>(.*?)</p>`)
	imageRE     = regexp.MustCompile(`(?is)<img[^>]+src="([^"]+)"`)
	anchorRE    = regexp.MustCompile(`(?is)<a[^>]+href="([^"]+)"`)
	markupRE    = regexp.MustCompile(`(?s)<[^>]+>`)
	spacesRE    = regexp.MustCompile(`\s+`)
)

// fileExtensions are the hrefs that point at an actual book file rather than
// at navigation chrome on the record page.
var fileExtensions = map[string]bool{
	"pdf": true, "epub": true, "mobi": true, "azw3": true, "djvu": true,
	"fb2": true, "cbz": true, "cbr": true, "chm": true, "txt": true,
	"lit": true, "doc": true, "docx": true, "rar": true, "zip": true,
}

// ParseRecord reads a /book/<id> page into a Record. recordURL is kept as the
// provenance URL and as the base for relative links.
func ParseRecord(body []byte, recordURL string) (Record, error) {
	page := string(body)
	record := Record{URL: recordURL}

	if match := titleTagRE.FindStringSubmatch(page); match != nil {
		record.Title = cleanText(match[1])
	}
	if record.Title == "" {
		return Record{}, fmt.Errorf("parse libgen record %s: page has no book title", recordURL)
	}

	for _, row := range metaRowRE.FindAllStringSubmatch(page, -1) {
		applyMetaRow(&record, cleanText(row[1]), cleanText(row[2]))
	}
	if match := descBlockRE.FindStringSubmatch(page); match != nil {
		record.Description = cleanText(match[1])
	}
	record.CoverURL = firstCoverURL(page, recordURL)
	record.DownloadURLs = collectDownloadURLs(page, recordURL)
	return record, nil
}

func applyMetaRow(record *Record, label, value string) {
	switch normalizeLabel(label) {
	case "author", "authors":
		record.Author = value
	case "publisher":
		record.Publisher = value
	case "year":
		record.Year = value
	case "language":
		record.Language = value
	case "extension":
		record.Extension = value
	case "filesize":
		record.Filesize = value
	case "pages":
		record.Pages = value
	case "isbn":
		record.ISBN = value
	case "isbn13":
		if record.ISBN == "" {
			record.ISBN = value
		}
	case "recordid":
		if id, err := strconv.ParseInt(value, 10, 64); err == nil {
			record.ID = id
		}
	}
}

// normalizeLabel lowercases and strips punctuation so "Author(s)" and
// "ISBN-13" match their plain spellings.
func normalizeLabel(label string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

// cleanText drops inline markup, decodes entities and collapses whitespace so
// values compare cleanly against the catalog.
func cleanText(fragment string) string {
	text := markupRE.ReplaceAllString(fragment, " ")
	text = html.UnescapeString(text)
	return strings.TrimSpace(spacesRE.ReplaceAllString(text, " "))
}

func firstCoverURL(page, recordURL string) string {
	for _, match := range imageRE.FindAllStringSubmatch(page, -1) {
		source := html.UnescapeString(match[1])
		if strings.Contains(source, "logo") || strings.Contains(source, "/assets/img/") {
			continue
		}
		if absolute := resolveURL(recordURL, source); absolute != "" {
			return absolute
		}
	}
	return ""
}

func collectDownloadURLs(page, recordURL string) []string {
	var links []string
	seen := map[string]bool{}
	for _, match := range anchorRE.FindAllStringSubmatch(page, -1) {
		href := html.UnescapeString(strings.TrimSpace(match[1]))
		if !looksLikeFileLink(href) {
			continue
		}
		absolute := resolveURL(recordURL, href)
		if absolute == "" || seen[absolute] {
			continue
		}
		seen[absolute] = true
		links = append(links, absolute)
	}
	return links
}

func looksLikeFileLink(href string) bool {
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") {
		return false
	}
	lower := strings.ToLower(href)
	if strings.Contains(lower, "get.php") || strings.Contains(lower, "/download/") || strings.Contains(lower, "/file/") {
		return true
	}
	path := lower
	if cut := strings.IndexAny(path, "?#"); cut >= 0 {
		path = path[:cut]
	}
	dot := strings.LastIndex(path, ".")
	return dot >= 0 && fileExtensions[path[dot+1:]]
}
