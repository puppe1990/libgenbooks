package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/puppe1990/libgenbooks/internal/annas"
	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/ipfs"
	"github.com/puppe1990/libgenbooks/internal/libgen"
	"github.com/puppe1990/libgenbooks/internal/sources"
)

func newTestSession(t *testing.T) (*mcp.ClientSession, *catalog.Store) {
	return newTestSessionWithMirror(t, "https://libgen.invalid")
}

func newTestSessionWithMirror(t *testing.T, baseURL string) (*mcp.ClientSession, *catalog.Store) {
	t.Helper()
	client := libgen.New(baseURL)
	client.DownloadDir = t.TempDir()
	return newTestSessionWith(t, Options{Libgen: client})
}

func newTestSessionWith(t *testing.T, options Options) (*mcp.ClientSession, *catalog.Store) {
	t.Helper()
	store, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	options.Catalog = store

	server := New(options)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "libgen-test", Version: "0.1.0"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, store
}

func textContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func TestServer_RegistersSearchAndDownloadTools(t *testing.T) {
	session, _ := newTestSession(t)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"libgen_search_books", "libgen_download_book"} {
		if !names[want] {
			t.Errorf("tool %q not registered (got %v)", want, names)
		}
	}
}

func TestServer_SearchBooks(t *testing.T) {
	session, store := newTestSession(t)
	if err := store.UpsertBook(catalog.Book{
		ID: 1, Title: "Harry Potter and the Philosopher's Stone",
		Author: "J K Rowling", Year: 1997, Extension: "epub", MD5: "abc", Searchable: 1,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "libgen_search_books",
		Arguments: map[string]any{"query": "harry potter", "limit": 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", textContent(t, result))
	}
	body := textContent(t, result)
	if !strings.Contains(body, "Harry Potter") || !strings.Contains(body, "Rowling") {
		t.Errorf("search output = %s", body)
	}
}

func TestServer_SearchBooksEmptyQueryIsToolError(t *testing.T) {
	session, _ := newTestSession(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "libgen_search_books",
		Arguments: map[string]any{"query": "   "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want a tool error (content %s)", textContent(t, result))
	}
}

func TestServer_DownloadBookByDirectURL(t *testing.T) {
	session, store := newTestSession(t)
	payload := []byte("EPUB-PAYLOAD")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	if err := store.UpsertBook(catalog.Book{
		ID: 7, Title: "Clean Code", Extension: "epub", MD5: "def", Searchable: 1,
	}); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "clean-code.epub")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "libgen_download_book",
		Arguments: map[string]any{
			"ref": "7",
			"url": srv.URL + "/clean.epub",
			"out": destination,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", textContent(t, result))
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("downloaded body = %q", got)
	}
	if body := textContent(t, result); !strings.Contains(body, "Clean Code") {
		t.Errorf("download output = %s", body)
	}
}

func TestServer_LiveSearchThenDownloadByMD5(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "libgen", "testdata", "search.html"))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("EPUB-FROM-MIRROR")
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write(fixture)
		case "/ads.php":
			_, _ = w.Write([]byte(`<a href="get.php?md5=184c16eae281bf2ebea72eb6d7aa6f6d&key=KEY1">GET</a>`))
		case "/get.php":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mirror.Close()

	session, store := newTestSessionWithMirror(t, mirror.URL)

	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "libgen_search_books",
		Arguments: map[string]any{"query": "harry potter", "limit": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if searchResult.IsError {
		t.Fatalf("search error: %s", textContent(t, searchResult))
	}
	if body := textContent(t, searchResult); !strings.Contains(body, "184c16eae281bf2ebea72eb6d7aa6f6d") {
		t.Fatalf("search output missing md5: %s", body)
	}
	// The live hit must be cached so a later download can resolve it by md5.
	if _, err := store.FindByMD5("184c16eae281bf2ebea72eb6d7aa6f6d"); err != nil {
		t.Fatalf("live hit was not cached: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "harry.epub")
	downloadResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "libgen_download_book",
		Arguments: map[string]any{
			"ref": "184c16eae281bf2ebea72eb6d7aa6f6d",
			"out": destination,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if downloadResult.IsError {
		t.Fatalf("download error: %s", textContent(t, downloadResult))
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("downloaded body = %q", got)
	}
}

func TestServer_DownloadUsesResolverChain(t *testing.T) {
	deadMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	t.Cleanup(deadMirror.Close)

	const docsCID = "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
	annasSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="https://ipfs.io/ipfs/` + docsCID + `">IPFS</a>`))
	}))
	t.Cleanup(annasSrv.Close)

	payload := []byte("EPUB-FROM-IPFS")
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/epub+zip")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(gateway.Close)

	client := libgen.New(deadMirror.URL)
	client.DownloadDir = t.TempDir()
	annasClient := annas.New("")
	annasClient.Domains = []string{annasSrv.URL}
	ipfsClient := ipfs.New()
	ipfsClient.Gateways = []string{gateway.URL}
	providers, err := sources.Providers([]string{sources.NameLibgen, sources.NameAnnas}, client, annasClient, ipfsClient)
	if err != nil {
		t.Fatal(err)
	}

	session, _ := newTestSessionWith(t, Options{Libgen: client, Resolver: sources.New(client, providers)})

	destination := filepath.Join(t.TempDir(), "harry.epub")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "libgen_download_book",
		Arguments: map[string]any{
			"ref": "184c16eae281bf2ebea72eb6d7aa6f6d",
			"out": destination,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("download error: %s", textContent(t, result))
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("downloaded body = %q", got)
	}
	if body := textContent(t, result); !strings.Contains(body, "annas") {
		t.Errorf("output = %s, want the annas provider named", body)
	}
}

func TestServer_DownloadUnknownRefIsToolError(t *testing.T) {
	session, _ := newTestSession(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "libgen_download_book",
		Arguments: map[string]any{"ref": "does-not-exist"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want a tool error (content %s)", textContent(t, result))
	}
}
