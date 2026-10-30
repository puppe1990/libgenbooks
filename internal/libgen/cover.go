package libgen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// maxCoverBytes caps a misparsed ads.php URL so a book file cannot stream
// through the cover proxy.
const maxCoverBytes = 2 << 20

// ResolveCover reads ads.php for md5 and returns the first same-host cover URL.
func (c *Client) ResolveCover(ctx context.Context, md5 string) (string, error) {
	md5, err := normalizeMD5(md5)
	if err != nil {
		return "", err
	}
	adsURL := fmt.Sprintf("%s/ads.php?md5=%s", c.BaseURL, md5)
	body, err := c.get(ctx, adsURL)
	if err != nil {
		return "", err
	}
	cover := firstCoverURL(string(body), adsURL)
	if cover == "" {
		return "", fmt.Errorf("libgen: no cover image for md5 %s (ads page %s)", md5, adsURL)
	}
	if !sameHost(c.BaseURL, cover) {
		return "", fmt.Errorf("libgen: cover host %s is not the mirror %s", cover, c.BaseURL)
	}
	return cover, nil
}

// FetchCover downloads the cover JPEG/PNG into DownloadDir/covers/<md5>.
func (c *Client) FetchCover(ctx context.Context, md5 string) (string, string, error) {
	md5, err := normalizeMD5(md5)
	if err != nil {
		return "", "", err
	}
	dest := c.coverPath(md5)
	if path, contentType, ok := readCachedCover(dest); ok {
		return path, contentType, nil
	}
	coverURL, err := c.ResolveCover(ctx, md5)
	if err != nil {
		return "", "", err
	}
	if err := c.fetchCoverFile(ctx, md5, coverURL, dest); err != nil {
		return "", "", err
	}
	return readCoverOrError(dest)
}

func readCachedCover(dest string) (string, string, bool) {
	data, err := os.ReadFile(dest)
	if err != nil || len(data) == 0 {
		return "", "", false
	}
	contentType := sniffImageType(data)
	if contentType == "" {
		return "", "", false
	}
	return dest, contentType, true
}

func readCoverOrError(dest string) (string, string, error) {
	data, err := os.ReadFile(dest)
	if err != nil {
		return "", "", fmt.Errorf("read cover %s: %w", dest, err)
	}
	contentType := sniffImageType(data)
	if contentType == "" {
		_ = os.Remove(dest)
		return "", "", fmt.Errorf("cover %s: body is not an image", dest)
	}
	return dest, contentType, nil
}

func (c *Client) fetchCoverFile(ctx context.Context, md5, coverURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return fmt.Errorf("build cover request for %s: %w", coverURL, err)
	}
	c.applyCoverHeaders(req, md5)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("download cover %s: %w", coverURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download cover %s: status %d", coverURL, resp.StatusCode)
	}
	return writeCoverBody(dest, coverURL, resp.Body)
}

func (c *Client) applyCoverHeaders(req *http.Request, md5 string) {
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "image/jpeg,image/png,image/webp,image/*;q=0.8")
	req.Header.Set("Referer", fmt.Sprintf("%s/ads.php?md5=%s", c.BaseURL, md5))
	if c.Cookie != "" {
		req.Header.Set("Cookie", c.Cookie)
	}
}

func writeCoverBody(dest, coverURL string, body io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create cover directory for %s: %w", dest, err)
	}
	written, err := writeAtomic(dest, io.LimitReader(body, maxCoverBytes+1))
	if err != nil {
		return err
	}
	if written > maxCoverBytes {
		_ = os.Remove(dest)
		return fmt.Errorf("cover %s: body larger than %d bytes", coverURL, maxCoverBytes)
	}
	return nil
}

func (c *Client) coverPath(md5 string) string {
	return filepath.Join(c.downloadDir(), "covers", md5)
}

func sameHost(baseURL, coverURL string) bool {
	base, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	cover, err := url.Parse(coverURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(base.Scheme, cover.Scheme) && strings.EqualFold(base.Host, cover.Host)
}

func sniffImageType(body []byte) string {
	switch {
	case bytes.HasPrefix(body, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(body, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png"
	case bytes.HasPrefix(body, []byte("GIF87a")), bytes.HasPrefix(body, []byte("GIF89a")):
		return "image/gif"
	case isWebP(body):
		return "image/webp"
	default:
		return ""
	}
}

func isWebP(body []byte) bool {
	return len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP"))
}

func normalizeMD5(md5 string) (string, error) {
	md5 = strings.ToLower(strings.TrimSpace(md5))
	if len(md5) != 32 || !allHex(md5) {
		return "", fmt.Errorf("invalid md5 %q: want 32 hex characters", md5)
	}
	return md5, nil
}
