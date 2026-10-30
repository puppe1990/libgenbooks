// Package libgen talks to a Library Genesis mirror: live search, the anonymous
// ads.php/get.php download handshake, record pages and the raw dump listing.
package libgen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the Library Genesis mirror the app talks to unless
// LIBGEN_BASE_URL overrides it. libgen.li serves anonymous search and the
// ads.php/get.php download handshake; libgen.ad only publishes record pages.
const DefaultBaseURL = "https://libgen.li"

// DefaultUserAgent identifies the app to the mirror.
const DefaultUserAgent = "libgen-books/0.1 (+https://github.com/puppe1990/libgenbooks)"

// Client fetches record pages and files from one Library Genesis mirror.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Cookie is the raw Cookie header copied from a logged-in browser session.
	// The mirror hides download links from anonymous record pages, so a session
	// cookie is what turns a record page into downloadable URLs.
	Cookie    string
	UserAgent string
	// DownloadDir is where derived filenames land (default "downloads").
	DownloadDir string
}

// New builds a client for baseURL (DefaultBaseURL when empty).
func New(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		UserAgent:   DefaultUserAgent,
		DownloadDir: "downloads",
	}
}

// Record fetches and parses the public record page for a libgen catalog id.
func (c *Client) Record(ctx context.Context, id int64) (Record, error) {
	if id <= 0 {
		return Record{}, fmt.Errorf("invalid libgen id %d", id)
	}
	recordURL := fmt.Sprintf("%s/book/%d", c.BaseURL, id)
	body, err := c.get(ctx, recordURL)
	if err != nil {
		return Record{}, err
	}
	return ParseRecord(body, recordURL)
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if c.Cookie != "" {
		req.Header.Set("Cookie", c.Cookie)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rawURL, err)
	}
	return body, nil
}

// resolveURL turns a record-page href into an absolute URL.
func resolveURL(baseURL, href string) string {
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return base.ResolveReference(parsed).String()
}
