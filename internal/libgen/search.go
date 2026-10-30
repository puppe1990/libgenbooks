package libgen

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Entry is one file row from the mirror's search table.
type Entry struct {
	ID           int64  `json:"id"`
	MD5          string `json:"md5"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	Publisher    string `json:"publisher"`
	Year         int    `json:"year"`
	Language     string `json:"language"`
	Pages        string `json:"pages"`
	Extension    string `json:"extension"`
	Filesize     int64  `json:"filesize"`
	FilesizeText string `json:"filesize_text"`
}

var (
	tableRowRE    = regexp.MustCompile(`(?is)<tr[^>]*>.*?</tr>`)
	tableCellRE   = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	editionLinkRE = regexp.MustCompile(`(?is)edition\.php\?id=\d+"[^>]*>(.*?)</a>`)
	boldTitleRE   = regexp.MustCompile(`(?is)<b>(.*?)</b>`)
	md5LinkRE     = regexp.MustCompile(`(?i)ads\.php\?md5=([0-9a-f]{32})`)
	fileIDRE      = regexp.MustCompile(`(?i)file\.php\?id=(\d+)`)
	yearRE        = regexp.MustCompile(`\b(1[5-9]\d{2}|20\d{2})\b`)
	sizeRE        = regexp.MustCompile(`(?i)^([0-9.,\s]+)\s*(b|kb|mb|gb|tb)$`)
)

var sizeUnits = map[string]int64{
	"b": 1, "kb": 1000, "mb": 1000 * 1000, "gb": 1000 * 1000 * 1000, "tb": 1000 * 1000 * 1000 * 1000,
}

// Search queries the mirror's live file search and returns up to limit rows,
// newest publication year first. This is the anonymous path: the search and
// download pages are public even though libgen.ad gates downloads behind an
// account.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Entry, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search libgen: empty query")
	}
	limit = clampSearchLimit(limit)

	searchURL := fmt.Sprintf("%s/index.php?req=%s&curtab=f&res=%d&page=1&order=year&ordermode=desc",
		c.BaseURL, url.QueryEscape(query), limit)
	body, err := c.get(ctx, searchURL)
	if err != nil {
		return nil, err
	}

	entries := ParseSearch(body, c.BaseURL)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// ParseSearch extracts the file rows from a mirror search results page.
func ParseSearch(body []byte, baseURL string) []Entry {
	page := string(body)
	start := strings.Index(page, "<tbody>")
	end := strings.Index(page, "</tbody>")
	if start < 0 || end <= start {
		return nil
	}

	var entries []Entry
	for _, row := range tableRowRE.FindAllString(page[start:end], -1) {
		entry, ok := parseSearchRow(row)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func parseSearchRow(row string) (Entry, bool) {
	md5Match := md5LinkRE.FindStringSubmatch(row)
	if md5Match == nil {
		return Entry{}, false
	}
	entry := Entry{MD5: strings.ToLower(md5Match[1])}
	if idMatch := fileIDRE.FindStringSubmatch(row); idMatch != nil {
		entry.ID, _ = strconv.ParseInt(idMatch[1], 10, 64)
	}

	cells := tableCellRE.FindAllStringSubmatch(row, -1)
	if len(cells) >= 8 {
		entry.Title = searchRowTitle(cells[0][1])
		entry.Author = cleanText(cells[1][1])
		entry.Publisher = cleanText(cells[2][1])
		entry.Year = parseYear(cleanText(cells[3][1]))
		entry.Language = cleanText(cells[4][1])
		entry.Pages = cleanText(cells[5][1])
		entry.FilesizeText = cleanText(cells[6][1])
		entry.Filesize = parseFilesize(entry.FilesizeText)
		entry.Extension = strings.ToLower(cleanText(cells[7][1]))
	}
	if entry.Title == "" {
		return Entry{}, false
	}
	return entry, true
}

func searchRowTitle(cell string) string {
	if match := editionLinkRE.FindStringSubmatch(cell); match != nil {
		if title := cleanText(match[1]); title != "" {
			return title
		}
	}
	if match := boldTitleRE.FindStringSubmatch(cell); match != nil {
		return cleanText(match[1])
	}
	return cleanText(cell)
}

func parseYear(text string) int {
	match := yearRE.FindString(text)
	if match == "" {
		return 0
	}
	year, _ := strconv.Atoi(match)
	return year
}

// parseFilesize turns the human cell ("342 kB", "19.8 MB") into bytes.
func parseFilesize(text string) int64 {
	match := sizeRE.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return 0
	}
	number := strings.NewReplacer(",", ".", " ", "").Replace(match[1])
	value, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0
	}
	return int64(value * float64(sizeUnits[strings.ToLower(match[2])]))
}

func clampSearchLimit(limit int) int {
	if limit <= 0 {
		return 25
	}
	if limit > 100 {
		return 100
	}
	return limit
}

const adsAttempts = 3

// ResolveDownload follows the ads page for an md5 and returns the signed
// get.php link that serves the file. Same-host only, so a poisoned ads.php
// cannot turn the web 302 into an open redirect. Extra DownloadMirrors are
// tried when BaseURL's ads.php keeps returning 5xx.
func (c *Client) ResolveDownload(ctx context.Context, md5 string) (string, error) {
	md5 = strings.ToLower(strings.TrimSpace(md5))
	if len(md5) != 32 || !allHex(md5) {
		return "", fmt.Errorf("invalid md5 %q: want 32 hex characters", md5)
	}
	var last error
	for _, base := range c.downloadBases() {
		link, err := c.resolveAds(ctx, base, md5)
		if err == nil {
			return link, nil
		}
		last = err
	}
	if last != nil {
		return "", last
	}
	return "", fmt.Errorf("libgen: no download mirror for md5 %s", md5)
}

// AdsPageURL is the ads.php landing page the browser can open when the
// server-side handshake fails. Prefers the first fallback mirror because
// libgen.li ads.php is the one that 500s.
func (c *Client) AdsPageURL(md5 string) string {
	md5 = strings.ToLower(strings.TrimSpace(md5))
	if len(md5) != 32 || !allHex(md5) {
		return ""
	}
	bases := c.downloadBases()
	if len(bases) == 0 {
		return ""
	}
	base := bases[0]
	if len(bases) > 1 {
		base = bases[1]
	}
	return base + "/ads.php?md5=" + md5
}

func (c *Client) downloadBases() []string {
	seen := make(map[string]bool)
	var bases []string
	add := func(raw string) {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
		bases = append(bases, raw)
	}
	add(c.BaseURL)
	for _, mirror := range c.DownloadMirrors {
		add(mirror)
	}
	return bases
}

func (c *Client) resolveAds(ctx context.Context, base, md5 string) (string, error) {
	adsURL := fmt.Sprintf("%s/ads.php?md5=%s", base, md5)
	body, err := c.getAds(ctx, adsURL)
	if err != nil {
		return "", err
	}
	link := findGetLink(body, adsURL)
	if link == "" {
		return "", fmt.Errorf("libgen: no download mirror for md5 %s (ads page %s)", md5, adsURL)
	}
	if !sameHost(base, link) {
		return "", fmt.Errorf("libgen: get.php host %s is not the mirror %s", link, base)
	}
	return link, nil
}

func (c *Client) getAds(ctx context.Context, adsURL string) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= adsAttempts; attempt++ {
		body, err := c.get(ctx, adsURL)
		if err == nil {
			return body, nil
		}
		last = err
		if !retryableFetch(err) {
			return nil, err
		}
	}
	return nil, last
}

func retryableFetch(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, code := range []string{"status 500", "status 502", "status 503", "status 429"} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return false
}

func findGetLink(body []byte, pageURL string) string {
	for _, match := range anchorRE.FindAllStringSubmatch(string(body), -1) {
		href := html.UnescapeString(strings.TrimSpace(match[1]))
		lower := strings.ToLower(href)
		if strings.Contains(lower, "get.php?") && strings.Contains(lower, "key=") {
			return resolveURL(pageURL, href)
		}
	}
	return ""
}

func allHex(text string) bool {
	for _, r := range text {
		if !isHexDigit(r) {
			return false
		}
	}
	return true
}

// isHexDigit spells the class as a switch so the negated compound does not
// trip staticcheck's De Morgan rewrite.
func isHexDigit(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'f':
		return true
	default:
		return false
	}
}
