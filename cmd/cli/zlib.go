package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/config"
	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

// runZlib searches Z-Library's JSON API and downloads a copy. The search is
// public; the signed file link needs the LIBGEN_ZLIB_* account.
func runZlib(ctx context.Context, cfg config.Config, args []string) error {
	flags := newFlagSet("zlib", "zlib <título|autor> [--pick N] [--out caminho] [--list]")
	pick := flags.Int("pick", 1, "número do resultado a baixar (1 = primeiro)")
	out := flags.String("out", "", "caminho do arquivo de destino")
	listOnly := flags.Bool("list", false, "só listar os resultados, sem baixar")
	positional, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(positional, " "))
	if query == "" {
		return errors.New("zlib: informe um título ou autor")
	}
	if !cfg.ZlibraryEnabled() {
		return errors.New("zlib: configure LIBGEN_ZLIB_EMAIL e LIBGEN_ZLIB_PASSWORD")
	}

	client := cfg.Zlibrary()
	books, err := client.Search(ctx, query, 1)
	if err != nil {
		return err
	}
	if len(books) == 0 {
		fmt.Println("nenhum resultado")
		return nil
	}
	printZlibBooks(books)
	if *listOnly {
		return nil
	}
	if *pick < 1 || *pick > len(books) {
		return fmt.Errorf("zlib: --pick %d fora dos %d resultados", *pick, len(books))
	}

	book := books[*pick-1]
	dest := strings.TrimSpace(*out)
	if dest == "" {
		dest = filepath.Join(cfg.DownloadDir, destFilename(book.Title, book.Extension, strings.ToLower(book.MD5)))
	}
	link, err := client.DownloadLink(ctx, book)
	if err != nil {
		return err
	}
	result, err := cfg.Client().FetchVerified(ctx, link, dest, book.MD5)
	if err != nil {
		return err
	}
	fmt.Printf("baixado: %s (%s) md5=%s (z-library)\n", result.Path, humanBytes(result.Bytes), result.MD5)
	return nil
}

// printZlibBooks lists the first hits so the reader can choose with --pick.
func printZlibBooks(books []zlibrary.Book) {
	limit := len(books)
	if limit > 10 {
		limit = 10
	}
	for index := 0; index < limit; index++ {
		book := books[index]
		shortMD5 := strings.ToLower(book.MD5)
		if len(shortMD5) > 8 {
			shortMD5 = shortMD5[:8]
		}
		fmt.Printf("%2d  %-8s %-5s %8s  %.70s\n", index+1, shortMD5, book.Extension, humanBytes(book.Filesize), book.Title)
	}
}
