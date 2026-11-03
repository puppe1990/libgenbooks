package annas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const (
	annaMD5 = "184c16eae281bf2ebea72eb6d7aa6f6d"
	docsCID = "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
)

func testClient(domains ...string) *Client {
	client := New("")
	client.Domains = domains
	return client
}

func fixtureServer(t *testing.T, file string) *httptest.Server {
	t.Helper()
	fixture, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCID_ReadsIPFSLinkFromMd5Page(t *testing.T) {
	var seenPath string
	fixture, err := os.ReadFile("testdata/md5_page.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	cid, err := testClient(srv.URL).CID(context.Background(), annaMD5)
	if err != nil {
		t.Fatal(err)
	}
	if cid != docsCID {
		t.Fatalf("cid = %q, want %q", cid, docsCID)
	}
	if seenPath != "/md5/"+annaMD5 {
		t.Fatalf("path = %q, want /md5/%s", seenPath, annaMD5)
	}
}

func TestCID_FallsBackToNextDomain(t *testing.T) {
	var primaryHits, secondaryHits int
	challenge, err := os.ReadFile("testdata/bot_challenge.html")
	if err != nil {
		t.Fatal(err)
	}
	record, err := os.ReadFile("testdata/md5_page.html")
	if err != nil {
		t.Fatal(err)
	}

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		_, _ = w.Write(challenge)
	}))
	t.Cleanup(primary.Close)

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryHits++
		_, _ = w.Write(record)
	}))
	t.Cleanup(secondary.Close)

	cid, err := testClient(primary.URL, secondary.URL).CID(context.Background(), annaMD5)
	if err != nil {
		t.Fatal(err)
	}
	if cid != docsCID {
		t.Fatalf("cid = %q, want %q", cid, docsCID)
	}
	if primaryHits != 1 || secondaryHits != 1 {
		t.Fatalf("hits = %d/%d, want the challenge domain tried once before the fallback", primaryHits, secondaryHits)
	}
}

func TestCID_NoCIDOnPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>Record</h1><p>No copies here.</p></body></html>`))
	}))
	t.Cleanup(srv.Close)

	_, err := testClient(srv.URL).CID(context.Background(), annaMD5)
	if err == nil || !strings.Contains(err.Error(), "ipfs cid") {
		t.Fatalf("err = %v, want an error about the missing ipfs cid", err)
	}
}

func TestCID_BotChallengeIsReported(t *testing.T) {
	srv := fixtureServer(t, "bot_challenge.html")
	_, err := testClient(srv.URL).CID(context.Background(), annaMD5)
	if err == nil || !strings.Contains(err.Error(), "bot challenge") {
		t.Fatalf("err = %v, want an error naming the bot challenge", err)
	}
}

func TestCID_RejectsBadMD5(t *testing.T) {
	if _, err := New("").CID(context.Background(), "nope"); err == nil {
		t.Fatal("CID(nope) = nil error, want error")
	}
}

func TestNew_PrimaryDomainLeadsRotation(t *testing.T) {
	client := New("https://annas-custom.example")
	if client.Domains[0] != "https://annas-custom.example" {
		t.Fatalf("Domains[0] = %q, want the primary override", client.Domains[0])
	}
	if len(client.Domains) != len(DefaultDomains)+1 {
		t.Fatalf("Domains = %v, want the rotation behind the primary", client.Domains)
	}
}
