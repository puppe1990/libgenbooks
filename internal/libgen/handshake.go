package libgen

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"
)

// adsAttempts is how many times one mirror's ads.php page is retried before
// that mirror counts as down for this handshake.
const adsAttempts = 3

// adsRetryDelay is the base pause between ads.php attempts; each attempt waits
// one more multiple of it so a rate-limited mirror gets breathing room. A
// variable so tests can drop the wait.
var adsRetryDelay = 200 * time.Millisecond

// adsMirrorTimeout bounds one mirror inside the race: a stalled host must lose
// to its siblings instead of holding the whole download.
const adsMirrorTimeout = 15 * time.Second

// ResolveDownload follows the ads page for an md5 and returns the signed
// get.php link that serves the file. Mirrors race in parallel and the first
// link wins. Same-host only, so a poisoned ads.php cannot turn the web 302
// into an open redirect.
func (c *Client) ResolveDownload(ctx context.Context, md5 string) (string, error) {
	md5 = strings.ToLower(strings.TrimSpace(md5))
	if len(md5) != 32 || !allHex(md5) {
		return "", fmt.Errorf("invalid md5 %q: want 32 hex characters", md5)
	}
	bases := c.mirrorBases()
	if len(bases) == 1 {
		return c.resolveAds(ctx, bases[0], md5)
	}
	if len(bases) == 0 {
		return "", fmt.Errorf("no download mirror for md5 %s", md5)
	}
	return c.raceAds(ctx, bases, md5)
}

// adsOutcome is one mirror's result inside the race.
type adsOutcome struct {
	base string
	link string
	err  error
}

// raceAds runs the ads handshake against every mirror at once and returns the
// first signed link; the slower mirrors are cancelled as soon as one wins.
func (c *Client) raceAds(ctx context.Context, bases []string, md5 string) (string, error) {
	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan adsOutcome, len(bases))
	var racers sync.WaitGroup
	for _, base := range bases {
		racers.Add(1)
		go func(base string) {
			defer racers.Done()
			attemptCtx, attemptCancel := context.WithTimeout(raceCtx, adsMirrorTimeout)
			defer attemptCancel()
			link, err := c.resolveAds(attemptCtx, base, md5)
			results <- adsOutcome{base: base, link: link, err: err}
		}(base)
	}
	go func() {
		racers.Wait()
		close(results)
	}()

	var failures []error
	for outcome := range results {
		if outcome.err == nil {
			cancel()
			return outcome.link, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", outcome.base, outcome.err))
	}
	return "", fmt.Errorf("no download mirror for md5 %s: %w", md5, errors.Join(failures...))
}

// AdsPageURL is the ads.php landing page the browser can open when the
// server-side handshake fails. Prefers the first fallback mirror because
// libgen.li ads.php is the one that 500s.
func (c *Client) AdsPageURL(md5 string) string {
	md5 = strings.ToLower(strings.TrimSpace(md5))
	if len(md5) != 32 || !allHex(md5) {
		return ""
	}
	bases := c.mirrorBases()
	if len(bases) == 0 {
		return ""
	}
	base := bases[0]
	if len(bases) > 1 {
		base = bases[1]
	}
	return base + "/ads.php?md5=" + md5
}

// mirrorBases is the ordered, deduped host list every mirror-facing operation
// walks: search and the download handshake share the same rotation.
func (c *Client) mirrorBases() []string {
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
		return "", fmt.Errorf("no get.php link for md5 %s (ads page %s)", md5, adsURL)
	}
	if !sameHost(base, link) {
		return "", fmt.Errorf("get.php host %s is not the mirror %s", link, base)
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
		if err := sleepBeforeRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}
	return nil, last
}

// retryableFetch reports whether the mirror's answer is a transient one worth
// another attempt: 5xx and 429. The check is typed, not matched on the message.
func retryableFetch(err error) bool {
	var statusErr statusError
	if !errors.As(err, &statusErr) {
		return false
	}
	switch statusErr.status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

// sleepBeforeRetry pauses between attempts, scaled by the attempt number, and
// gives up as soon as the context is done.
func sleepBeforeRetry(ctx context.Context, attempt int) error {
	if adsRetryDelay <= 0 {
		return nil
	}
	timer := time.NewTimer(adsRetryDelay * time.Duration(attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
