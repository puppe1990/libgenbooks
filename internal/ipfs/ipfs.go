// Package ipfs resolves a CID into working public gateway URLs, the
// mirror-free layer for files Anna's Archive exposes over IPFS.
package ipfs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultGateways are the public HTTP gateways raced for a CID. Public
// gateways are slow and flaky on their own, so they only ever race for the
// first answer instead of being treated as a primary source.
var DefaultGateways = []string{
	"https://ipfs.io",
	"https://dweb.link",
	"https://w3s.link",
	"https://4everland.io",
	"https://nftstorage.link",
	"https://gateway.pinata.cloud",
}

// DefaultUserAgent identifies the app to the gateways; empty agents are
// rejected by most of them.
const DefaultUserAgent = "libgen-books/0.2 (+https://github.com/puppe1990/libgenbooks)"

// probeTimeout bounds one gateway probe: a gateway that cannot deliver the
// first bytes within it loses the race.
const probeTimeout = 6 * time.Second

// probeBytes is the range the probe asks for, enough to prove the gateway
// streams the file and not a placeholder page.
const probeBytes = 512

// Client races a set of public IPFS gateways for one CID.
type Client struct {
	Gateways  []string
	HTTP      *http.Client
	UserAgent string
}

// New builds a client over the default gateway set.
func New() *Client {
	return &Client{
		Gateways:  DefaultGateways,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		UserAgent: DefaultUserAgent,
	}
}

// URLFor joins one gateway root with a CID.
func URLFor(gateway, cid string) string {
	return strings.TrimRight(gateway, "/") + "/ipfs/" + cid
}

// Resolve races every gateway and returns their file URLs for cid, the first
// gateway that served the first bytes first; the remaining URLs follow as
// fetch fallbacks. It fails when no gateway answers.
func (c *Client) Resolve(ctx context.Context, cid string) ([]string, error) {
	cid = strings.TrimSpace(cid)
	if !LooksLikeCID(cid) {
		return nil, fmt.Errorf("ipfs: %q is not a CID", cid)
	}
	gateways := c.gatewayList()
	if len(gateways) == 0 {
		return nil, fmt.Errorf("ipfs: no gateway configured for cid %s", cid)
	}
	winner, err := c.race(ctx, cid, gateways)
	if err != nil {
		return nil, err
	}
	return orderedURLs(gateways, winner, cid), nil
}

func (c *Client) race(ctx context.Context, cid string, gateways []string) (string, error) {
	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	winners := make(chan string, len(gateways))
	var racers sync.WaitGroup
	for _, gateway := range gateways {
		racers.Add(1)
		go func(gateway string) {
			defer racers.Done()
			attemptCtx, attemptCancel := context.WithTimeout(raceCtx, probeTimeout)
			defer attemptCancel()
			if c.serves(attemptCtx, URLFor(gateway, cid)) {
				winners <- gateway
			}
		}(gateway)
	}
	go func() {
		racers.Wait()
		close(winners)
	}()

	if gateway, ok := <-winners; ok {
		cancel()
		return gateway, nil
	}
	return "", fmt.Errorf("ipfs: no gateway served cid %s", cid)
}

// serves reports whether one gateway delivered the first byte of the file; a
// status-200 HTML page is an error page, not a file.
func (c *Client) serves(ctx context.Context, rawURL string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	request.Header.Set("User-Agent", c.userAgent())
	request.Header.Set("Range", fmt.Sprintf("bytes=0-%d", probeBytes-1))
	response, err := c.httpClient().Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return false
	}
	if htmlPayload(response.Header.Get("Content-Type")) {
		return false
	}
	_, err = io.CopyN(io.Discard, response.Body, 1)
	return err == nil
}

// orderedURLs lists every gateway URL for cid, the race winner first.
func orderedURLs(gateways []string, winner, cid string) []string {
	urls := make([]string, 0, len(gateways))
	urls = append(urls, URLFor(winner, cid))
	for _, gateway := range gateways {
		if gateway == winner {
			continue
		}
		urls = append(urls, URLFor(gateway, cid))
	}
	return urls
}

// LooksLikeCID accepts CIDv0 (Qm…, base58, 46 chars) and the base32 CIDv1
// forms (bafy…/bafk…) that Anna's Archive links expose.
func LooksLikeCID(cid string) bool {
	if len(cid) == 46 && strings.HasPrefix(cid, "Qm") {
		return allRunes(cid[2:], isBase58)
	}
	if strings.HasPrefix(cid, "bafy") || strings.HasPrefix(cid, "bafk") {
		return len(cid) >= 55 && allRunes(cid, isBase32Lower)
	}
	return false
}

func allRunes(text string, accept func(rune) bool) bool {
	for _, r := range text {
		if !accept(r) {
			return false
		}
	}
	return true
}

// isBase58 matches base58btc: no 0, O, I or l.
func isBase58(r rune) bool {
	switch {
	case r >= '1' && r <= '9':
		return true
	case r >= 'A' && r <= 'H':
		return true
	case r >= 'J' && r <= 'N':
		return true
	case r >= 'P' && r <= 'Z':
		return true
	case r >= 'a' && r <= 'k':
		return true
	case r >= 'm' && r <= 'z':
		return true
	default:
		return false
	}
}

func isBase32Lower(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= '2' && r <= '7':
		return true
	default:
		return false
	}
}

func htmlPayload(contentType string) bool {
	lower := strings.ToLower(contentType)
	return strings.Contains(lower, "text/html") || strings.Contains(lower, "application/xhtml")
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

func (c *Client) gatewayList() []string {
	gateways := make([]string, 0, len(c.Gateways))
	for _, gateway := range c.Gateways {
		if trimmed := strings.TrimRight(strings.TrimSpace(gateway), "/"); trimmed != "" {
			gateways = append(gateways, trimmed)
		}
	}
	return gateways
}
