package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

const harryMD5 = "184c16eae281bf2ebea72eb6d7aa6f6d"

type fakeProvider struct {
	name string
	urls []string
	err  error
}

func (p fakeProvider) Name() string { return p.name }

func (p fakeProvider) URLs(context.Context, string) ([]string, error) { return p.urls, p.err }

type fakeFetcher struct {
	fail       map[string]bool
	fetched    []string
	verifiedAs []string
}

func (f *fakeFetcher) FetchVerified(_ context.Context, rawURL, destPath, wantMD5 string) (libgen.Result, error) {
	f.fetched = append(f.fetched, rawURL)
	f.verifiedAs = append(f.verifiedAs, wantMD5)
	if f.fail[rawURL] {
		return libgen.Result{}, fmt.Errorf("fetch %s: boom", rawURL)
	}
	return libgen.Result{URL: rawURL, Path: destPath, Bytes: 3}, nil
}

func (f *fakeFetcher) Destination(request libgen.DownloadRequest) string {
	return request.DestPath
}

func TestChain_DownloadWalksProvidersUntilOneWorks(t *testing.T) {
	fetcher := &fakeFetcher{fail: map[string]bool{"https://mirror.example/get.php?key=K": true}}
	chain := New(fetcher, []Provider{
		fakeProvider{name: "libgen", urls: []string{"https://mirror.example/get.php?key=K"}},
		fakeProvider{name: "annas", urls: []string{"https://gateway.example/ipfs/CID"}},
	})

	result, err := chain.Download(context.Background(), libgen.DownloadRequest{MD5: harryMD5, DestPath: "/tmp/book.epub"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "annas" {
		t.Errorf("Source = %q, want the fallback provider", result.Source)
	}
	if result.URL != "https://gateway.example/ipfs/CID" {
		t.Errorf("URL = %q", result.URL)
	}
	if result.Path != "/tmp/book.epub" || result.Bytes != 3 {
		t.Errorf("result = %+v", result)
	}
	for _, want := range fetcher.verifiedAs {
		if want != harryMD5 {
			t.Errorf("FetchVerified got wantMD5 %q, want the requested %s", want, harryMD5)
		}
	}
}

func TestChain_DownloadTriesNextURLWithinProvider(t *testing.T) {
	fetcher := &fakeFetcher{fail: map[string]bool{"https://gateway.example/slow": true}}
	chain := New(fetcher, []Provider{
		fakeProvider{name: "annas", urls: []string{"https://gateway.example/slow", "https://gateway.example/fast"}},
	})

	result, err := chain.Download(context.Background(), libgen.DownloadRequest{MD5: harryMD5, DestPath: "/tmp/book.epub"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://gateway.example/fast" {
		t.Errorf("URL = %q, want the second gateway", result.URL)
	}
	if result.Source != "annas" {
		t.Errorf("Source = %q", result.Source)
	}
}

func TestChain_DownloadReportsEveryProviderFailure(t *testing.T) {
	fetcher := &fakeFetcher{fail: map[string]bool{"https://mirror.example/get.php?key=K": true}}
	chain := New(fetcher, []Provider{
		fakeProvider{name: "libgen", urls: []string{"https://mirror.example/get.php?key=K"}},
		fakeProvider{name: "annas", err: fmt.Errorf("no ipfs cid")},
	})

	_, err := chain.Download(context.Background(), libgen.DownloadRequest{MD5: harryMD5, DestPath: "/tmp/book.epub"})
	if err == nil {
		t.Fatal("Download() = nil error, want the joined failure")
	}
	for _, want := range []string{"libgen", "annas", harryMD5} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestChain_DownloadNeedsMD5(t *testing.T) {
	chain := New(&fakeFetcher{}, []Provider{fakeProvider{name: "libgen"}})
	_, err := chain.Download(context.Background(), libgen.DownloadRequest{DestPath: "/tmp/book.epub"})
	if err == nil || !strings.Contains(err.Error(), "md5") {
		t.Fatalf("err = %v, want an error naming the missing md5", err)
	}
}

func TestChain_ResolveReturnsFirstWorkingProvider(t *testing.T) {
	chain := New(&fakeFetcher{}, []Provider{
		fakeProvider{name: "libgen", err: fmt.Errorf("ads.php 500")},
		fakeProvider{name: "annas", urls: []string{"https://gateway.example/ipfs/CID", "https://backup.example/ipfs/CID"}},
	})

	link, err := chain.Resolve(context.Background(), harryMD5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if link.Provider != "annas" {
		t.Errorf("Provider = %q", link.Provider)
	}
	if link.URL != "https://gateway.example/ipfs/CID" {
		t.Errorf("URL = %q, want the first candidate", link.URL)
	}
}

func TestChain_ResolveSkipsRejectedCandidates(t *testing.T) {
	chain := New(&fakeFetcher{}, []Provider{
		fakeProvider{name: "libgen", urls: []string{"https://mirror.example/get.php?key=K"}},
		fakeProvider{name: "annas", urls: []string{"https://gateway.one/ipfs/CID", "https://gateway.two/ipfs/CID"}},
	})

	link, err := chain.Resolve(context.Background(), harryMD5, func(_ context.Context, candidate Link) bool {
		return candidate.URL == "https://gateway.two/ipfs/CID"
	})
	if err != nil {
		t.Fatal(err)
	}
	if link.Provider != "annas" || link.URL != "https://gateway.two/ipfs/CID" {
		t.Fatalf("link = %+v, want the first accepted candidate", link)
	}
}

func TestChain_ResolveAllRejectedIsAnError(t *testing.T) {
	chain := New(&fakeFetcher{}, []Provider{
		fakeProvider{name: "libgen", urls: []string{"https://mirror.example/get.php?key=K"}},
	})

	_, err := chain.Resolve(context.Background(), harryMD5, func(context.Context, Link) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("err = %v, want the rejection reported", err)
	}
}

func TestChain_ResolveReportsEveryProviderFailure(t *testing.T) {
	chain := New(&fakeFetcher{}, []Provider{
		fakeProvider{name: "libgen", err: fmt.Errorf("ads.php 500")},
		fakeProvider{name: "annas", err: fmt.Errorf("no ipfs cid")},
	})

	_, err := chain.Resolve(context.Background(), harryMD5, nil)
	if err == nil || !strings.Contains(err.Error(), "libgen") || !strings.Contains(err.Error(), "annas") {
		t.Fatalf("err = %v, want both providers named", err)
	}
}
