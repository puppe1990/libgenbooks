package libgen

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// peekTimeout is short on purpose: a blocked file CDN must not stall the
// web 302. Unsynced HTML 500s come back from Cloudflare in well under this.
var peekTimeout = 2500 * time.Millisecond

// GetLinkUnsynced reports whether the file CDN answered with the "repositories
// may not be synchronized" HTML. Timeouts, 403s and real file responses
// return false so the browser still tries with a residential IP. HEAD only:
// never pull the book through the app.
func (c *Client) GetLinkUnsynced(ctx context.Context, getURL string) bool {
	getURL = strings.TrimSpace(getURL)
	if getURL == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, peekTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, getURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", c.peekUA())
	if c.BaseURL != "" {
		req.Header.Set("Referer", strings.TrimRight(c.BaseURL, "/")+"/")
	}

	resp, err := c.peekClient().Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	return unsyncedCDNResponse(resp.StatusCode, resp.Header.Get("Content-Type"))
}

func (c *Client) peekUA() string {
	if strings.TrimSpace(c.UserAgent) != "" {
		return c.UserAgent
	}
	return DefaultUserAgent
}

func (c *Client) peekClient() *http.Client {
	transport := http.DefaultTransport
	if c.HTTP != nil && c.HTTP.Transport != nil {
		transport = c.HTTP.Transport
	}
	return &http.Client{Transport: transport, Timeout: peekTimeout}
}

func unsyncedCDNResponse(status int, contentType string) bool {
	if status != http.StatusInternalServerError && status != http.StatusNotFound {
		return false
	}
	lower := strings.ToLower(contentType)
	return strings.Contains(lower, "text/html") || lower == ""
}
