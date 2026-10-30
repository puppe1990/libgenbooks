package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/config"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func runDownload(ctx context.Context, cfg config.Config, args []string) error {
	flags := newFlagSet("download", "download <id|md5|título> [--out caminho] [--url URL]")
	out := flags.String("out", "", "caminho do arquivo de destino")
	directURL := flags.String("url", "", "URL direta do arquivo (pula o handshake do espelho)")
	positional, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	reference := strings.TrimSpace(strings.Join(positional, " "))
	if reference == "" && *directURL == "" {
		return errors.New("download: informe um id, md5, título ou --url")
	}

	store, err := catalog.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	book, md5, err := resolveDownloadRef(store, reference, *directURL)
	if err != nil {
		return err
	}
	result, err := cfg.Client().Download(ctx, libgen.DownloadRequest{
		BookID:    book.ID,
		MD5:       md5,
		Title:     book.Title,
		Extension: book.Extension,
		DirectURL: *directURL,
		DestPath:  *out,
	})
	if err != nil {
		return err
	}
	fmt.Printf("baixado: %s (%s)\n", result.Path, humanBytes(result.Bytes))
	return nil
}

// resolveDownloadRef maps the CLI reference to a catalog row and the md5 the
// mirror handshake needs. A direct URL makes the catalog lookup optional.
func resolveDownloadRef(store *catalog.Store, reference, directURL string) (catalog.Book, string, error) {
	if reference == "" {
		if directURL == "" {
			return catalog.Book{}, "", errors.New("download: informe um id, md5, título ou --url")
		}
		return catalog.Book{}, "", nil
	}
	if catalog.IsMD5(reference) {
		book, err := store.FindByMD5(strings.ToLower(reference))
		if err != nil {
			return catalog.Book{}, strings.ToLower(reference), nil
		}
		return book, strings.ToLower(reference), nil
	}

	book, err := store.FindByRef(reference)
	if err == nil {
		return book, bookHash(book), nil
	}
	if directURL != "" {
		return catalog.Book{ID: numericRef(reference)}, "", nil
	}
	return catalog.Book{}, "", err
}

func bookHash(book catalog.Book) string {
	if catalog.IsMD5(book.MD5) {
		return strings.ToLower(book.MD5)
	}
	return ""
}

func numericRef(reference string) int64 {
	id, err := strconv.ParseInt(reference, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
