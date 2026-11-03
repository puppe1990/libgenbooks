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

// Fetcher persists one file URL; *libgen.Client implements it.
type Fetcher interface {
	FetchFile(ctx context.Context, rawURL, destPath string) (libgen.Result, error)
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
		result, err := c.fetcher.FetchFile(ctx, rawURL, destPath)
		if err == nil {
			result.Source = provider.Name()
			return result, nil
		}
		failures = append(failures, err)
	}
	return libgen.Result{}, fmt.Errorf("%s: %w", provider.Name(), errors.Join(failures...))
}

// Resolve returns the first provider's preferred file URL without fetching
// the file, for callers that hand the URL to someone else (the web 302).
func (c *Chain) Resolve(ctx context.Context, md5 string) (Link, error) {
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
		return Link{Provider: provider.Name(), URL: urls[0]}, nil
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
