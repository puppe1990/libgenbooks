package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/cache"
	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/config"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func runSearch(ctx context.Context, cfg config.Config, args []string) error {
	flags := newFlagSet("search", "search <título|autor|isbn> [--limit N] [--local]")
	limit := flags.Int("limit", 10, "máximo de resultados")
	localOnly := flags.Bool("local", false, "buscar só no catálogo local")
	positional, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(positional, " "))
	if query == "" {
		return errors.New("search: informe o texto a procurar")
	}
	if *localOnly {
		return searchLocal(cfg, query, *limit)
	}

	entries, err := cfg.Client().Search(ctx, query, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aviso: espelho indisponível (%v); buscando no catálogo local\n", err)
		return searchLocal(cfg, query, *limit)
	}
	if len(entries) == 0 {
		fmt.Println("nenhum resultado")
		return nil
	}
	cacheEntries(cfg, entries)
	printEntries(os.Stdout, entries)
	fmt.Fprintf(os.Stdout, "\npara baixar: libgen-books download <%s|md5|título>\n", "id")
	return nil
}

// cacheEntries persists live hits best-effort: a read-only catalog must not
// hide the search output.
func cacheEntries(cfg config.Config, entries []libgen.Entry) {
	store, err := catalog.Open(cfg.DBPath)
	if err != nil {
		return
	}
	defer func() { _ = store.Close() }()
	cache.SaveEntries(store, entries)
}

func searchLocal(cfg config.Config, query string, limit int) error {
	store, err := catalog.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	books, err := store.Search(query, limit)
	if err != nil {
		return err
	}
	if len(books) == 0 {
		fmt.Println("nenhum resultado no catálogo local (importe um snapshot com `libgen-books import`)")
		return nil
	}
	printEntries(os.Stdout, entriesFromBooks(books))
	return nil
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
