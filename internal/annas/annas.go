// Package annas reads Anna's Archive md5 records for the IPFS CID of a book,
// the bridge to mirror-free downloads through public gateways.
package annas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/puppe1990/libgenbooks/internal/ipfs"
)

// DefaultDomains are the Anna's Archive hosts; .gl is primary and .pk/.gd are
// the mirrors published alongside it. All of them sit behind a bot check that
// sometimes answers with a challenge page instead of the record.
var DefaultDomains = []string{
	"https://annas-archive.gl",
	"https://annas-archive.pk",
	"https://annas-archive.gd",
}

// DefaultUserAgent identifies the app to Anna's Archive; empty agents are
// rejected outright.
const DefaultUserAgent = "libgen-books/0.2 (+https://github.com/puppe1990/libgenbooks)"

// domainTimeout bounds one host so a blocked domain falls through quickly.
const domainTimeout = 12 * time.Second

var (
	ipfsLinkRE  = regexp.MustCompile(`(?i)(?:/ipfs/|ipfs://)([A-Za-z0-9]{40,})`)
	cidTextRE   = regexp.MustCompile(`\b(bafy[a-z2-7]{50,}|Qm[1-9A-HJ-NP-Za-km-z]{44})\b`)
	challengeRE = regexp.MustCompile(`(?i)ddos-guard|checking your browser|cf-browser-verification`)
)

// Client reads Anna's Archive md5 pages across its domain rotation.
type Client struct {
	Domains   []string
	HTTP      *http.Client
	UserAgent string
}

// New builds a client whose first domain is primary (LIBGEN_ANNAS_URL),
// keeping the built-in rotation after it.
func New(primary string) *Client {
	primary = strings.TrimRight(strings.TrimSpace(primary), "/")
	domains := DefaultDomains
	if primary != "" && primary != DefaultDomains[0] {
		domains = append([]string{primary}, withoutDomain(DefaultDomains, primary)...)
	}
	return &Client{
		Domains:   domains,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		UserAgent: DefaultUserAgent,
	}
}

// CID returns the IPFS CID Anna's Archive lists for md5, trying each domain
// until one serves the record.
func (c *Client) CID(ctx context.Context, md5 string) (string, error) {
	md5 = strings.ToLower(strings.TrimSpace(md5))
	if len(md5) != 32 || !allHex(md5) {
		return "", fmt.Errorf("annas: invalid md5 %q: want 32 hex characters", md5)
	}
	var failures []error
	for _, domain := range c.domainList() {
		cid, err := c.cidFromDomain(ctx, domain, md5)
		if err == nil {
			return cid, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", domain, err))
	}
	return "", fmt.Errorf("no ipfs cid for md5 %s: %w", md5, errors.Join(failures...))
}

func (c *Client) cidFromDomain(ctx context.Context, domain, md5 string) (string, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, domainTimeout)
	defer cancel()

	body, err := c.get(attemptCtx, domain+"/md5/"+md5)
	if err != nil {
		return "", err
	}
	page := string(body)
	if challengeRE.MatchString(page) {
		return "", errors.New("served a bot challenge instead of the md5 page")
	}
	cid := extractCID(page)
	if cid == "" {
		return "", errors.New("the md5 page lists no ipfs cid")
	}
	return cid, nil
}

// extractCID reads the first IPFS link (or bare CID) on the md5 page.
func extractCID(page string) string {
	for _, match := range ipfsLinkRE.FindAllStringSubmatch(page, -1) {
		if cid := match[1]; ipfs.LooksLikeCID(cid) {
			return cid
		}
	}
	if match := cidTextRE.FindStringSubmatch(page); match != nil {
		return match[1]
	}
	return ""
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", rawURL, err)
	}
	request.Header.Set("User-Agent", c.userAgent())
	request.Header.Set("Accept", "text/html,application/xhtml+xml")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", rawURL, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rawURL, err)
	}
	return body, nil
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

func (c *Client) domainList() []string {
	domains := make([]string, 0, len(c.Domains))
	for _, domain := range c.Domains {
		if trimmed := strings.TrimRight(strings.TrimSpace(domain), "/"); trimmed != "" {
			domains = append(domains, trimmed)
		}
	}
	return domains
}

func withoutDomain(domains []string, skip string) []string {
	kept := make([]string, 0, len(domains))
	for _, domain := range domains {
		if domain != skip {
			kept = append(kept, domain)
		}
	}
	return kept
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
