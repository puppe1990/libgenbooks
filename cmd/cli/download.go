package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/config"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func runDownload(ctx context.Context, cfg config.Config, args []string) error {
	flags := newFlagSet("download", "download <id|md5|título> [--out caminho] [--url URL] [--retry N] [--retry-interval 1m]")
	out := flags.String("out", "", "caminho do arquivo de destino")
	directURL := flags.String("url", "", "URL direta do arquivo (pula o handshake do espelho)")
	retries := flags.Int("retry", 1, "tentativas quando o CDN ainda não sincronizou o arquivo")
	retryInterval := flags.Duration("retry-interval", time.Minute, "pausa base entre tentativas (cresce a cada uma)")
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
	request := libgen.DownloadRequest{
		BookID:    book.ID,
		MD5:       md5,
		Title:     book.Title,
		Extension: book.Extension,
		DirectURL: *directURL,
		DestPath:  *out,
	}
	result, err := retryUnsynced(ctx, retryOptions{
		attempts: *retries,
		interval: *retryInterval,
		notice: func(attempt int, pause time.Duration) {
			fmt.Fprintf(os.Stderr, "%s ainda não sincronizou no CDN; nova tentativa em %s\n", shortRef(request.MD5), pause)
		},
	}, func(ctx context.Context) (libgen.Result, error) {
		return downloadViaChain(ctx, cfg, request)
	})
	if err != nil {
		return err
	}
	fmt.Printf("baixado: %s (%s)%s\n", result.Path, humanBytes(result.Bytes), sourceNote(result))
	return nil
}

// shortRef names the book in progress lines: the first md5 characters, or
// the direct URL when that is what we are fetching.
func shortRef(md5 string) string {
	if md5 == "" {
		return "download"
	}
	if len(md5) > 8 {
		return md5[:8]
	}
	return md5
}

// downloadViaChain walks the LIBGEN_SOURCES providers when the reference has
// an md5; direct URLs and id-only references keep the plain client path.
func downloadViaChain(ctx context.Context, cfg config.Config, request libgen.DownloadRequest) (libgen.Result, error) {
	if strings.TrimSpace(request.DirectURL) != "" || strings.TrimSpace(request.MD5) == "" {
		return cfg.Client().Download(ctx, request)
	}
	chain, err := cfg.Resolver()
	if err != nil {
		return libgen.Result{}, err
	}
	return chain.Download(ctx, request)
}

func sourceNote(result libgen.Result) string {
	if result.Source == "" {
		return ""
	}
	return " via " + result.Source
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
