// Package zlibrary talks to Z-Library's JSON API. The 1lib.sk website is
// behind DiamWall for programmatic clients, but the API host it redirects to
// (library-redirect.sk) answers plain HTTP JSON: search is public, and the
// signed file link only needs the account cookie from /eapi/user/login.
package zlibrary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is where api.z-lib.gd/eapi redirects land; calling it
// directly keeps the query string intact (the redirect appends "?books").
const DefaultBaseURL = "https://library-redirect.sk"

// DefaultUserAgent identifies the app to the API.
const DefaultUserAgent = "libgen-books/0.2 (+https://github.com/puppe1990/libgenbooks)"

// maxResponseBytes caps one API answer.
const maxResponseBytes = 8 << 20

// Book is one search result. The API also returns sha256, language and more;
// only what the downloader needs is decoded.
type Book struct {
	ID        int64  `json:"id"`
	Hash      string `json:"hash"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Extension string `json:"extension"`
	Filesize  int64  `json:"filesize"`
	// MD5 is the file hash Z-Library reports — the downloader verifies
	// against it, since these copies usually differ from the libgen file.
	MD5 string `json:"md5"`
}

// Client talks to one Z-Library API host.
type Client struct {
	BaseURL   string
	Email     string
	Password  string
	HTTP      *http.Client
	UserAgent string

	mu      sync.Mutex
	loginMu sync.Mutex
	session *session
}

type session struct {
	userID  int64
	userKey string
	quota   Quota
	quotaAt time.Time
}

// Quota is the account's daily download allowance as the API reports it.
type Quota struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

// quotaTTL bounds how long one quota reading is reused; a download marks it
// stale right away, and the timer covers changes made elsewhere.
const quotaTTL = 5 * time.Minute

// errPleaseLogin marks an expired session, so the caller can log in again.
var errPleaseLogin = errors.New("zlibrary: please login")

// ErrQuotaExhausted marks the account's daily allowance being spent. Callers
// surface a friendly message instead of the API's raw English, and must not
// retry — the limit only resets on the next day boundary.
var ErrQuotaExhausted = errors.New("zlibrary: daily download limit reached")

// quotaPhrases match the API's wordings for an exhausted daily allowance.
// The match stays narrow: a false negative only shows the raw message, while
// a false positive would mislabel an unrelated failure.
var quotaPhrases = []string{
	"daily limit",
	"limit reached",
	"limit exceeded",
	"reached your limit",
	"wait until tomorrow",
}

func quotaExhausted(message string) bool {
	lower := strings.ToLower(message)
	for _, phrase := range quotaPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// New builds a client for baseURL (DefaultBaseURL when empty).
func New(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		UserAgent: DefaultUserAgent,
	}
}

// Search finds books by title or author. The query rides in the URL
// (?message=); the body only carries the page. This endpoint is public.
func (c *Client) Search(ctx context.Context, query string, page int) ([]Book, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("zlibrary: empty search query")
	}
	if page < 1 {
		page = 1
	}
	body, err := json.Marshal(map[string]int{"page": page})
	if err != nil {
		return nil, fmt.Errorf("zlibrary: encode search body: %w", err)
	}
	rawURL := fmt.Sprintf("%s/eapi/book/search?message=%s", c.BaseURL, url.QueryEscape(query))
	payload, err := c.do(ctx, http.MethodPost, rawURL, "application/json", body, nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Success int    `json:"success"`
		Error   string `json:"error"`
		Books   []Book `json:"books"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, fmt.Errorf("zlibrary: decode search response: %w", err)
	}
	if response.Success != 1 {
		return nil, apiFailure("search", response.Error)
	}
	if response.Books == nil {
		return []Book{}, nil
	}
	return response.Books, nil
}

// DownloadLink returns the signed file URL for one book. It logs in on first
// use and again when the API answers "Please login" (expired key).
func (c *Client) DownloadLink(ctx context.Context, book Book) (string, error) {
	if book.ID <= 0 || strings.TrimSpace(book.Hash) == "" {
		return "", fmt.Errorf("zlibrary: book %d has no id/hash to resolve", book.ID)
	}
	link, err := c.fileLink(ctx, book)
	if err == nil {
		c.markQuotaStale()
		return link, nil
	}
	if !errors.Is(err, errPleaseLogin) {
		return "", err
	}
	c.forgetSession()
	link, err = c.fileLink(ctx, book)
	if err == nil {
		c.markQuotaStale()
	}
	return link, err
}

// Quota returns the account's daily download allowance, re-reading it when
// the cached reading aged out or a download just consumed one.
func (c *Client) Quota(ctx context.Context) (Quota, error) {
	if quota, ok := c.cachedQuota(); ok {
		return quota, nil
	}
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if quota, ok := c.cachedQuota(); ok {
		return quota, nil
	}
	s, err := c.login(ctx)
	if err != nil {
		return Quota{}, err
	}
	c.storeSession(s)
	return s.quota, nil
}

func (c *Client) fileLink(ctx context.Context, book Book) (string, error) {
	session, err := c.sessionFor(ctx)
	if err != nil {
		return "", err
	}
	rawURL := fmt.Sprintf("%s/eapi/book/%d/%s/file", c.BaseURL, book.ID, url.PathEscape(book.Hash))
	cookie := fmt.Sprintf("remix_userid=%d; remix_userkey=%s", session.userID, session.userKey)
	payload, err := c.do(ctx, http.MethodGet, rawURL, "", nil, map[string]string{"Cookie": cookie})
	if err != nil {
		return "", err
	}
	var response struct {
		Success int    `json:"success"`
		Error   string `json:"error"`
		File    struct {
			DownloadLink string `json:"downloadLink"`
		} `json:"file"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return "", fmt.Errorf("zlibrary: decode file response: %w", err)
	}
	if response.Success != 1 {
		lower := strings.ToLower(response.Error)
		if strings.Contains(lower, "please login") {
			return "", errPleaseLogin
		}
		if quotaExhausted(lower) {
			return "", fmt.Errorf("%w: %s", ErrQuotaExhausted, strings.TrimSpace(response.Error))
		}
		return "", apiFailure("file", response.Error)
	}
	if strings.TrimSpace(response.File.DownloadLink) == "" {
		return "", fmt.Errorf("zlibrary: file response has no downloadLink")
	}
	return response.File.DownloadLink, nil
}

// sessionFor returns the cached session, logging in when there is none. The
// login runs outside c.mu — holding it across the network call would block
// Quota and every other download behind one slow login.
func (c *Client) sessionFor(ctx context.Context) (session, error) {
	if cached, ok := c.cachedSession(); ok {
		return cached, nil
	}
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if cached, ok := c.cachedSession(); ok {
		return cached, nil
	}
	s, err := c.login(ctx)
	if err != nil {
		return session{}, err
	}
	c.storeSession(s)
	return s, nil
}

func (c *Client) cachedSession() (session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return session{}, false
	}
	return *c.session, true
}

// cachedQuota returns the cached reading while it is still fresh.
func (c *Client) cachedQuota() (Quota, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil || time.Since(c.session.quotaAt) >= quotaTTL {
		return Quota{}, false
	}
	return c.session.quota, true
}

func (c *Client) storeSession(s session) {
	c.mu.Lock()
	c.session = &s
	c.mu.Unlock()
}

// login performs one login. The endpoint only accepts form-encoded
// credentials — a JSON body answers "Incorrect email or password". Both
// credentials are trimmed: a stray space pasted from .env would otherwise
// fail the login with no visible cause. Only sessionFor and Quota call it,
// and only while holding loginMu, so logins never stack.
func (c *Client) login(ctx context.Context) (session, error) {
	email := strings.TrimSpace(c.Email)
	password := strings.TrimSpace(c.Password)
	if email == "" || password == "" {
		return session{}, fmt.Errorf("zlibrary: login needs LIBGEN_ZLIB_EMAIL and LIBGEN_ZLIB_PASSWORD")
	}
	form := url.Values{"email": {email}, "password": {password}, "remember": {"true"}}.Encode()
	payload, err := c.do(ctx, http.MethodPost, c.BaseURL+"/eapi/user/login", "application/x-www-form-urlencoded", []byte(form), nil)
	if err != nil {
		return session{}, err
	}
	var response struct {
		Success int    `json:"success"`
		Error   string `json:"error"`
		User    struct {
			ID             int64  `json:"id"`
			RemixUserKey   string `json:"remix_userkey"`
			DownloadsToday int    `json:"downloads_today"`
			DownloadsLimit int    `json:"downloads_limit"`
		} `json:"user"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return session{}, fmt.Errorf("zlibrary: decode login response: %w", err)
	}
	if response.Success != 1 {
		return session{}, apiFailure("login", response.Error)
	}
	if response.User.ID <= 0 || strings.TrimSpace(response.User.RemixUserKey) == "" {
		return session{}, fmt.Errorf("zlibrary: login response missing user id or remix_userkey")
	}
	return session{
		userID:  response.User.ID,
		userKey: strings.TrimSpace(response.User.RemixUserKey),
		quota:   Quota{Used: response.User.DownloadsToday, Limit: response.User.DownloadsLimit},
		quotaAt: time.Now(),
	}, nil
}

func (c *Client) forgetSession() {
	c.mu.Lock()
	c.session = nil
	c.mu.Unlock()
}

// markQuotaStale forces the next Quota call to re-read the account, since a
// download just consumed one from the daily allowance.
func (c *Client) markQuotaStale() {
	c.mu.Lock()
	if c.session != nil {
		c.session.quotaAt = time.Time{}
	}
	c.mu.Unlock()
}

// do runs one API request. The API reports business failures as JSON
// (success=0) on any status, so callers parse the body; a non-2xx answer
// whose body is not JSON at all (an edge node's HTML error page) is reported
// with its status instead of surfacing as a baffling decode error.
func (c *Client) do(ctx context.Context, method, rawURL, contentType string, body []byte, headers map[string]string) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, fmt.Errorf("zlibrary: build request for %s: %w", rawURL, err)
	}
	request.Header.Set("User-Agent", c.userAgent())
	request.Header.Set("Accept", "application/json")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("zlibrary: fetch %s: %w", rawURL, err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("zlibrary: read %s: %w", rawURL, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if !json.Valid(payload) {
			return nil, fmt.Errorf("zlibrary: fetch %s: status %d with a non-JSON body", rawURL, response.StatusCode)
		}
	}
	return payload, nil
}

func apiFailure(op, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "request failed"
	}
	return fmt.Errorf("zlibrary: %s: %s", op, message)
}

func (c *Client) userAgent() string {
	if strings.TrimSpace(c.UserAgent) != "" {
		return c.UserAgent
	}
	return DefaultUserAgent
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
