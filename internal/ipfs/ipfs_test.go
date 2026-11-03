package ipfs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	docsCID   = "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
	legacyCID = "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
)

func TestLooksLikeCID(t *testing.T) {
	cases := map[string]bool{
		docsCID:   true,
		legacyCID: true,
		"nope":    false,
		"bafy":    false,
		"QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbd":               false,
		"bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdI": false,
	}
	for input, want := range cases {
		if got := LooksLikeCID(input); got != want {
			t.Errorf("LooksLikeCID(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestResolve_RejectsBadCID(t *testing.T) {
	_, err := New().Resolve(context.Background(), "not-a-cid")
	if err == nil || !strings.Contains(err.Error(), "not-a-cid") {
		t.Fatalf("err = %v, want an error naming the bad cid", err)
	}
}

func TestResolve_PicksFirstGatewayThatAnswers(t *testing.T) {
	htmlGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>missing</html>"))
	}))
	t.Cleanup(htmlGateway.Close)

	fileGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ipfs/"+docsCID {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/epub+zip")
		_, _ = w.Write([]byte("EPUBDATA"))
	}))
	t.Cleanup(fileGateway.Close)

	client := New()
	client.Gateways = []string{htmlGateway.URL, fileGateway.URL}
	urls, err := client.Resolve(context.Background(), docsCID)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 2 {
		t.Fatalf("urls = %v, want both gateways", urls)
	}
	if urls[0] != fileGateway.URL+"/ipfs/"+docsCID {
		t.Fatalf("urls[0] = %q, want the serving gateway first", urls[0])
	}
}

func TestResolve_FastGatewayBeatsSlowOne(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		w.Header().Set("Content-Type", "application/epub+zip")
		_, _ = w.Write([]byte("EPUBDATA"))
	}))
	t.Cleanup(slow.Close)

	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/epub+zip")
		_, _ = w.Write([]byte("EPUBDATA"))
	}))
	t.Cleanup(fast.Close)

	client := New()
	client.Gateways = []string{slow.URL, fast.URL}
	start := time.Now()
	urls, err := client.Resolve(context.Background(), docsCID)
	if err != nil {
		t.Fatal(err)
	}
	if urls[0] != fast.URL+"/ipfs/"+docsCID {
		t.Fatalf("urls[0] = %q, want the fast gateway", urls[0])
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("Resolve took %v: gateways must race in parallel", elapsed)
	}
}

func TestResolve_NoGatewayAnswers(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(dead.Close)

	client := New()
	client.Gateways = []string{dead.URL}
	_, err := client.Resolve(context.Background(), docsCID)
	if err == nil || !strings.Contains(err.Error(), docsCID) {
		t.Fatalf("err = %v, want an error naming cid %s", err, docsCID)
	}
}
