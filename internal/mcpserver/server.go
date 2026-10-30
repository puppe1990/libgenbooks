// Package mcpserver exposes the libgen catalog and the mirror downloader as MCP
// tools, so an agent can search and fetch a book in one stdio session.
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

// Options are the dependencies shared by every tool handler.
type Options struct {
	Catalog *catalog.Store
	Libgen  *libgen.Client
}

// New builds the MCP server with the search and download tools registered.
func New(options Options) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "libgen-books", Version: "0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "libgen_search_books",
		Description: "Search Library Genesis by title, author or ISBN. Queries the mirror live and caches every hit in the local catalog for offline use.",
	}, searchBooksHandler(options))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "libgen_download_book",
		Description: "Download one book by catalog id, md5 or title. Uses the anonymous mirror handshake; pass a direct file url when you already have one.",
	}, downloadBookHandler(options))

	return server
}

// Serve runs the MCP server on stdio until ctx is cancelled.
func Serve(ctx context.Context, options Options) error {
	return New(options).Run(ctx, &mcp.StdioTransport{})
}
