package mcpserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/puppe1990/libgenbooks/internal/cache"
	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

const (
	defaultSearchLimit = 10
	maxSearchLimit     = 100
)

type searchInput struct {
	Query string `json:"query" jsonschema:"title, author, ISBN or free text to look up"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of results (default 10, max 100)"`
}

type searchOutput struct {
	Books  []libgen.Entry `json:"books"`
	Count  int            `json:"count"`
	Source string         `json:"source"`
	Hint   string         `json:"hint,omitempty"`
}

func searchBooksHandler(options Options) mcp.ToolHandlerFor[searchInput, searchOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
		query := strings.TrimSpace(input.Query)
		if query == "" {
			return nil, searchOutput{}, fmt.Errorf("query is required: pass a title, author or ISBN")
		}
		limit := clampLimit(input.Limit)

		entries, liveErr := options.Libgen.Search(ctx, query, limit)
		if liveErr == nil && len(entries) > 0 {
			cache.SaveEntries(options.Catalog, entries)
			return nil, searchOutput{Books: entries, Count: len(entries), Source: "mirror"}, nil
		}
		if liveErr == nil {
			return nil, searchOutput{Books: []libgen.Entry{}, Source: "mirror", Hint: "mirror returned no rows for this query"}, nil
		}

		books, cacheErr := options.Catalog.Search(query, limit)
		if cacheErr != nil {
			return nil, searchOutput{}, fmt.Errorf("mirror search failed (%v) and the local catalog failed (%w)", liveErr, cacheErr)
		}
		if len(books) == 0 {
			return nil, searchOutput{}, fmt.Errorf("mirror search failed (%v) and the local catalog has no match; run `libgen-books import <snapshot>` for offline search", liveErr)
		}
		return nil, searchOutput{
			Books:  entriesFromBooks(books),
			Count:  len(books),
			Source: "local catalog",
			Hint:   "mirror unavailable: " + liveErr.Error(),
		}, nil
	}
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}

func entriesFromBooks(books []catalog.Book) []libgen.Entry {
	entries := make([]libgen.Entry, 0, len(books))
	for _, book := range books {
		entries = append(entries, libgen.Entry{
			ID:        book.ID,
			MD5:       book.MD5,
			Title:     book.Title,
			Author:    book.Author,
			Year:      book.Year,
			Extension: book.Extension,
			Filesize:  book.Filesize,
		})
	}
	return entries
}

type downloadInput struct {
	Ref string `json:"ref" jsonschema:"catalog id, md5 hash or title returned by libgen_search_books"`
	URL string `json:"url,omitempty" jsonschema:"direct file URL; skips the mirror download handshake"`
	Out string `json:"out,omitempty" jsonschema:"destination path (default downloads/<title>.<extension>)"`
}

type downloadOutput struct {
	ID    int64  `json:"id"`
	MD5   string `json:"md5,omitempty"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func downloadBookHandler(options Options) mcp.ToolHandlerFor[downloadInput, downloadOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input downloadInput) (*mcp.CallToolResult, downloadOutput, error) {
		book, md5, err := resolveTarget(options.Catalog, input.Ref, input.URL)
		if err != nil {
			return nil, downloadOutput{}, err
		}
		result, err := options.Libgen.Download(ctx, libgen.DownloadRequest{
			BookID:    book.ID,
			MD5:       md5,
			Title:     book.Title,
			Extension: book.Extension,
			DirectURL: input.URL,
			DestPath:  input.Out,
		})
		if err != nil {
			return nil, downloadOutput{}, err
		}
		return nil, downloadOutput{
			ID:    book.ID,
			MD5:   md5,
			Title: book.Title,
			URL:   result.URL,
			Path:  result.Path,
			Bytes: result.Bytes,
		}, nil
	}
}

// resolveTarget maps a user reference to a catalog row plus the md5 needed by
// the download handshake. A direct URL makes the lookup best-effort: the caller
// may be fetching an edition the local catalog does not carry yet.
func resolveTarget(store *catalog.Store, ref, directURL string) (catalog.Book, string, error) {
	reference := strings.TrimSpace(ref)
	if catalog.IsMD5(reference) {
		book, err := store.FindByMD5(reference)
		if err != nil {
			return catalog.Book{}, reference, nil
		}
		return book, reference, nil
	}
	if reference == "" {
		if strings.TrimSpace(directURL) == "" {
			return catalog.Book{}, "", fmt.Errorf("ref is required: pass an id, md5 or title from libgen_search_books")
		}
		return catalog.Book{}, "", nil
	}
	book, err := store.FindByRef(reference)
	if err == nil {
		return book, bookMD5(book), nil
	}
	if strings.TrimSpace(directURL) != "" {
		return catalog.Book{ID: numericID(reference)}, "", nil
	}
	return catalog.Book{}, "", err
}

func bookMD5(book catalog.Book) string {
	if catalog.IsMD5(book.MD5) {
		return book.MD5
	}
	return ""
}

func numericID(ref string) int64 {
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
