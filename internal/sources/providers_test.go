package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/libgenbooks/internal/annas"
	"github.com/puppe1990/libgenbooks/internal/ipfs"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

const docsCID = "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"

func namesOf(providers []Provider) []string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name())
	}
	return names
}

func TestProviders_DefaultOrderIsLibgenThenAnnas(t *testing.T) {
	providers, err := Providers(nil, libgen.New("https://libgen.invalid"), annas.New(""), ipfs.New())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(namesOf(providers), ",")
	if got != "libgen,annas" {
		t.Fatalf("providers = %s, want libgen,annas", got)
	}
}

func TestProviders_UnknownNameIsLoud(t *testing.T) {
	_, err := Providers([]string{"megaupload"}, libgen.New("https://libgen.invalid"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "megaupload") || !strings.Contains(err.Error(), "libgen") {
		t.Fatalf("err = %v, want the bad name plus the valid ones", err)
	}
}

func TestProviders_AnnasNeedsItsClients(t *testing.T) {
	_, err := Providers([]string{"annas"}, libgen.New("https://libgen.invalid"), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "annas") {
		t.Fatalf("err = %v, want an error naming the missing annas/ipfs clients", err)
	}
}

func TestLibgenProvider_ReturnsTheHandshakeLink(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="get.php?md5=` + harryMD5 + `&key=KEY123">GET</a>`))
	}))
	t.Cleanup(mirror.Close)

	providers, err := Providers([]string{"libgen"}, libgen.New(mirror.URL), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	urls, err := providers[0].URLs(context.Background(), harryMD5)
	if err != nil {
		t.Fatal(err)
	}
	want := mirror.URL + "/get.php?md5=" + harryMD5 + "&key=KEY123"
	if len(urls) != 1 || urls[0] != want {
		t.Fatalf("urls = %v, want [%s]", urls, want)
	}
}

func TestAnnasProvider_ResolvesCIDThroughGateways(t *testing.T) {
	md5Page := `<html><body><a href="https://ipfs.io/ipfs/` + docsCID + `">IPFS Gateway #1</a></body></html>`
	annasSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/md5/"+harryMD5 {
			t.Errorf("annas path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(md5Page))
	}))
	t.Cleanup(annasSrv.Close)

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/epub+zip")
		_, _ = w.Write([]byte("EPUBDATA"))
	}))
	t.Cleanup(gateway.Close)

	annasClient := annas.New("")
	annasClient.Domains = []string{annasSrv.URL}
	ipfsClient := ipfs.New()
	ipfsClient.Gateways = []string{gateway.URL}

	providers, err := Providers([]string{"annas"}, libgen.New("https://libgen.invalid"), annasClient, ipfsClient)
	if err != nil {
		t.Fatal(err)
	}
	urls, err := providers[0].URLs(context.Background(), harryMD5)
	if err != nil {
		t.Fatal(err)
	}
	want := ipfs.URLFor(gateway.URL, docsCID)
	if len(urls) == 0 || urls[0] != want {
		t.Fatalf("urls = %v, want [%s …]", urls, want)
	}
}
