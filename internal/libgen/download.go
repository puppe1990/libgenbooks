package libgen

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DownloadRequest names the book to fetch and where to put it. DirectURL wins
// over the md5 handshake, which wins over the record page. DestPath defaults to
// <DownloadDir>/<slug>.<extension>.
type DownloadRequest struct {
	BookID    int64
	MD5       string
	Title     string
	Extension string
	DirectURL string
	DestPath  string
}

// Result describes one finished file transfer.
type Result struct {
	URL   string `json:"url"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	// Source names the provider that produced the file; set by the source
	// chain, empty when the client fetched the URL directly.
	Source string `json:"source,omitempty"`
}

var unsafeFilenameRE = regexp.MustCompile(`[^a-z0-9._-]+`)

// Download streams the book file to DestPath. Without a DirectURL it reads the
// record page and uses the first link the page exposes, which requires the
// logged-in cookie the mirror gates downloads behind. When the request names
// an md5, the finished file must hash to it.
func (c *Client) Download(ctx context.Context, request DownloadRequest) (Result, error) {
	sourceURL, err := c.downloadSourceURL(ctx, request)
	if err != nil {
		return Result{}, err
	}
	return c.FetchVerified(ctx, sourceURL, c.Destination(request), request.MD5)
}

// Destination is where Download writes: request.DestPath when set, otherwise
// the derived <DownloadDir>/<slug>.<extension> (md5 when the title is unknown).
func (c *Client) Destination(request DownloadRequest) string {
	if strings.TrimSpace(request.DestPath) != "" {
		return request.DestPath
	}
	return filepath.Join(c.downloadDir(), filenameFor(request))
}

// downloadSourceURL picks the most direct route the caller gave us: an explicit
// file URL, the anonymous md5 handshake, then the record page.
func (c *Client) downloadSourceURL(ctx context.Context, request DownloadRequest) (string, error) {
	if direct := strings.TrimSpace(request.DirectURL); direct != "" {
		return direct, nil
	}
	if md5 := strings.TrimSpace(request.MD5); md5 != "" {
		return c.ResolveDownload(ctx, md5)
	}
	if request.BookID > 0 {
		return c.pickDownloadURL(ctx, request.BookID)
	}
	return "", fmt.Errorf("download: need an md5, a libgen id or a direct file URL")
}

// FetchFile streams an arbitrary URL to disk; the dump command uses it for the
// catalog snapshot, which has no record page. The body is not verified — there
// is no expected hash for a dump part.
func (c *Client) FetchFile(ctx context.Context, rawURL, destPath string) (Result, error) {
	if strings.TrimSpace(rawURL) == "" {
		return Result{}, fmt.Errorf("fetch file: empty URL")
	}
	written, _, err := c.fetchFile(ctx, rawURL, destPath)
	if err != nil {
		return Result{}, err
	}
	return Result{URL: rawURL, Path: destPath, Bytes: written}, nil
}

// FetchVerified is FetchFile plus the md5 gate: when wantMD5 is a 32-hex hash
// and the body does not hash to it, the file is removed and the fetch fails —
// a truncated or swapped file must not pass as a successful download.
func (c *Client) FetchVerified(ctx context.Context, rawURL, destPath, wantMD5 string) (Result, error) {
	if strings.TrimSpace(rawURL) == "" {
		return Result{}, fmt.Errorf("fetch file: empty URL")
	}
	written, sum, err := c.fetchFile(ctx, rawURL, destPath)
	if err != nil {
		return Result{}, err
	}
	if want := expectedMD5(wantMD5); want != "" && want != sum {
		_ = os.Remove(destPath)
		return Result{}, fmt.Errorf("download %s: md5 mismatch: computed %s, want %s", rawURL, sum, want)
	}
	return Result{URL: rawURL, Path: destPath, Bytes: written}, nil
}

func (c *Client) pickDownloadURL(ctx context.Context, id int64) (string, error) {
	record, err := c.Record(ctx, id)
	if err != nil {
		return "", err
	}
	if len(record.DownloadURLs) > 0 {
		return record.DownloadURLs[0], nil
	}
	hint := "set LIBGEN_COOKIE to the cookie of a logged-in session on the mirror"
	if c.Cookie != "" {
		hint = "LIBGEN_COOKIE is set but the record page exposed no link; refresh that cookie"
	}
	return "", fmt.Errorf("libgen: no download link for book %d (%s): %s, or pass a direct file URL", id, record.URL, hint)
}

// fetchFile streams rawURL to destPath and returns the bytes written plus the
// md5 of the body, hashed on the fly so callers can gate the result on the
// expected hash without a second read.
func (c *Client) fetchFile(ctx context.Context, rawURL, destPath string) (int64, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, "", fmt.Errorf("build download request for %s: %w", rawURL, err)
	}
	request.Header.Set("User-Agent", c.UserAgent)
	request.Header.Set("Referer", c.BaseURL+"/")
	if c.Cookie != "" {
		request.Header.Set("Cookie", c.Cookie)
	}

	response, err := c.HTTP.Do(request)
	if err != nil {
		return 0, "", fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, "", fmt.Errorf("download %s: status %d", rawURL, response.StatusCode)
	}
	if htmlPayload(response.Header.Get("Content-Type")) {
		return 0, "", fmt.Errorf("download %s: server answered with an HTML page instead of a file (content-type %q)", rawURL, response.Header.Get("Content-Type"))
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, "", fmt.Errorf("create download directory for %s: %w", destPath, err)
	}
	hash := md5.New()
	written, err := writeAtomic(destPath, io.TeeReader(response.Body, hash))
	if err != nil {
		return 0, "", err
	}
	return written, hex.EncodeToString(hash.Sum(nil)), nil
}

// writeAtomic writes to <dest>.part and renames, so an interrupted transfer
// never leaves a truncated book under its final name.
func writeAtomic(destPath string, body io.Reader) (int64, error) {
	partPath := destPath + ".part"
	file, err := os.Create(partPath)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", partPath, err)
	}
	written, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(partPath)
		return 0, fmt.Errorf("write %s: %w", partPath, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(partPath)
		return 0, fmt.Errorf("close %s: %w", partPath, closeErr)
	}
	if err := os.Rename(partPath, destPath); err != nil {
		_ = os.Remove(partPath)
		return 0, fmt.Errorf("rename %s to %s: %w", partPath, destPath, err)
	}
	return written, nil
}

func (c *Client) downloadDir() string {
	if strings.TrimSpace(c.DownloadDir) != "" {
		return c.DownloadDir
	}
	return "downloads"
}

func filenameFor(request DownloadRequest) string {
	name := filenameStem(request)
	extension := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(request.Extension)), ".")
	if extension == "" {
		return name
	}
	return name + "." + extension
}

// filenameStem prefers the title, then the catalog id, then the md5, so an
// md5-only download never lands as "libgen-0".
func filenameStem(request DownloadRequest) string {
	if name := slugify(request.Title); name != "" {
		return name
	}
	if request.BookID > 0 {
		return fmt.Sprintf("libgen-%d", request.BookID)
	}
	if md5 := strings.ToLower(strings.TrimSpace(request.MD5)); md5 != "" {
		return md5
	}
	return "libgen-book"
}

// htmlPayload reports the error pages some mirrors serve with status 200.
// A "successful" response must not land on disk as if it were a book.
func htmlPayload(contentType string) bool {
	lower := strings.ToLower(contentType)
	return strings.Contains(lower, "text/html") || strings.Contains(lower, "application/xhtml")
}

// expectedMD5 returns the lowercase hash to verify against, or "" when the
// reference is not an md5 — callers skip verification in that case (dumps and
// covers have no expected hash).
func expectedMD5(text string) string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if len(normalized) != 32 || !allHex(normalized) {
		return ""
	}
	return normalized
}

func slugify(title string) string {
	slug := strings.ToLower(strings.TrimSpace(title))
	slug = unsafeFilenameRE.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-._")
	if len(slug) > 120 {
		slug = strings.Trim(slug[:120], "-._")
	}
	return slug
}
