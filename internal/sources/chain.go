// Package sources resolves an md5 into a downloadable file across independent
// providers, so a dead mirror is not a dead end: the LibGen handshake runs
// first and Anna's Archive over IPFS gateways follows.
package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

// Provider turns an md5 into candidate file URLs, best first.
type Provider interface {
	Name() string
	URLs(ctx context.Context, md5 string) ([]string, error)
}

// Fetcher persists one file URL and verifies it against the expected md5;
// *libgen.Client implements it.
type Fetcher interface {
	FetchVerified(ctx context.Context, rawURL, destPath, wantMD5 string) (libgen.Result, error)
	Destination(request libgen.DownloadRequest) string
}

// Link is one resolved file URL and the provider that produced it.
type Link struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
}

// Chain runs the providers in order until one yields a working file.
type Chain struct {
	providers []Provider
	fetcher   Fetcher
}

// New assembles the chain; providers run in the given order.
func New(fetcher Fetcher, providers []Provider) *Chain {
	return &Chain{fetcher: fetcher, providers: providers}
}

// Download fetches md5's file to request.DestPath (or the derived path),
// walking the providers until one works. DirectURL is ignored: direct URLs
// belong to libgen.Client.Download.
func (c *Chain) Download(ctx context.Context, request libgen.DownloadRequest) (libgen.Result, error) {
	md5 := strings.ToLower(strings.TrimSpace(request.MD5))
	if md5 == "" {
		return libgen.Result{}, fmt.Errorf("sources: download needs an md5, got none")
	}
	destination := c.fetcher.Destination(request)

	var failures []error
	for _, provider := range c.providers {
		result, err := c.fetchFromProvider(ctx, provider, md5, destination)
		if err == nil {
			return result, nil
		}
		failures = append(failures, err)
	}
	return libgen.Result{}, fmt.Errorf("sources: every provider failed for md5 %s: %w", md5, errors.Join(failures...))
}

func (c *Chain) fetchFromProvider(ctx context.Context, provider Provider, md5, destPath string) (libgen.Result, error) {
	urls, err := candidateURLs(ctx, provider, md5)
	if err != nil {
		return libgen.Result{}, err
	}

	var failures []error
	for _, rawURL := range urls {
		result, err := c.fetcher.FetchVerified(ctx, rawURL, destPath, md5)
		if err == nil {
			result.Source = provider.Name()
			return result, nil
		}
		failures = append(failures, err)
	}
	return libgen.Result{}, fmt.Errorf("%s: %w", provider.Name(), errors.Join(failures...))
}

// Resolve returns the first candidate file URL the accept callback approves,
// walking every candidate of every provider. Callers use accept to reject a
// link the browser cannot use (an unsynced file CDN, for instance) so the
// chain keeps walking; nil accepts every candidate.
func (c *Chain) Resolve(ctx context.Context, md5 string, accept func(context.Context, Link) bool) (Link, error) {
	md5 = strings.ToLower(strings.TrimSpace(md5))
	if md5 == "" {
		return Link{}, fmt.Errorf("sources: resolve needs an md5, got none")
	}

	var failures []error
	for _, provider := range c.providers {
		urls, err := candidateURLs(ctx, provider, md5)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, rawURL := range urls {
			link := Link{Provider: provider.Name(), URL: rawURL}
			if accept == nil || accept(ctx, link) {
				return link, nil
			}
			failures = append(failures, fmt.Errorf("%s: rejected %s", provider.Name(), rawURL))
		}
	}
	return Link{}, fmt.Errorf("sources: every provider failed for md5 %s: %w", md5, errors.Join(failures...))
}

func candidateURLs(ctx context.Context, provider Provider, md5 string) ([]string, error) {
	urls, err := provider.URLs(ctx, md5)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", provider.Name(), err)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("%s: no candidate file urls for md5 %s", provider.Name(), md5)
	}
	return urls, nil
}
