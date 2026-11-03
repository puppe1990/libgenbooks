package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/config"
	"github.com/puppe1990/libgenbooks/internal/libgen"
	"github.com/puppe1990/libgenbooks/internal/sources"
)

// alternateTries caps how many replacement copies one pass attempts, so a
// hopeless title does not turn into a mirror crawl.
const alternateTries = 3

// pendingBook is one failed download the retry command has to fix.
type pendingBook struct {
	md5       string
	title     string
	author    string
	year      int
	extension string
	filesize  int64
	filename  string
	// userID names the history row to flip on success; 0 when the list came
	// from --from (there is no row to update).
	userID int64
}

type retryFailedOptions struct {
	destDir    string
	retries    int
	retryEvery time.Duration
	allowAlt   bool
	loop       bool
	pause      time.Duration
}

func runRetryFailed(ctx context.Context, cfg config.Config, args []string) error {
	flags := newFlagSet("retry-failed", "retry-failed [--user N] [--from lista.txt] [--out pasta] [--retry N] [--retry-interval 1m] [--no-alt] [--loop] [--pause 15m]")
	userID := flags.Int64("user", 0, "id do usuário no histórico (0 = todos)")
	fromFile := flags.String("from", "", "lista md5|arquivo por linha, em vez do histórico")
	out := flags.String("out", "", "pasta de destino (padrão LIBGEN_DOWNLOAD_DIR)")
	retries := flags.Int("retry", 1, "tentativas por livro quando o CDN ainda não sincronizou")
	retryInterval := flags.Duration("retry-interval", time.Minute, "pausa base entre tentativas (cresce a cada uma)")
	noAlt := flags.Bool("no-alt", false, "não tentar uma cópia alternativa do mesmo título")
	loop := flags.Bool("loop", false, "repete as passadas até baixar tudo")
	pause := flags.Duration("pause", 15*time.Minute, "pausa entre passadas com --loop")
	positional, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return fmt.Errorf("retry-failed: argumento inesperado %q", positional[0])
	}

	store, err := catalog.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	pending, err := collectPending(store, *fromFile, *userID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		fmt.Println("nenhum download pendente")
		return nil
	}

	destDir := strings.TrimSpace(*out)
	if destDir == "" {
		destDir = cfg.DownloadDir
	}
	chain, err := cfg.Resolver()
	if err != nil {
		return err
	}
	opts := retryFailedOptions{
		destDir:    destDir,
		retries:    *retries,
		retryEvery: *retryInterval,
		allowAlt:   !*noAlt,
		loop:       *loop,
		pause:      *pause,
	}
	fmt.Printf("%d download(s) para tentar em %s\n", len(pending), destDir)
	return runRetryPasses(ctx, cfg, store, chain, pending, opts)
}

// collectPending reads the failed books from --from when given, otherwise
// from the catalog history (every user when userID is 0).
func collectPending(store *catalog.Store, fromFile string, userID int64) ([]pendingBook, error) {
	if strings.TrimSpace(fromFile) != "" {
		return pendingFromFile(fromFile)
	}
	records, err := store.FailedDownloads(userID)
	if err != nil {
		return nil, err
	}
	books := make([]pendingBook, 0, len(records))
	for _, rec := range records {
		books = append(books, pendingBook{
			md5:       rec.MD5,
			title:     rec.Title,
			author:    rec.Author,
			year:      rec.Year,
			extension: rec.Extension,
			filesize:  rec.Filesize,
			filename:  destFilename(rec.Title, rec.Extension, rec.MD5),
			userID:    rec.UserID,
		})
	}
	return books, nil
}

// pendingFromFile reads "md5|arquivo" lines (plain md5 also accepted; blank
// and # lines ignored). Without a file name the md5 becomes the stem.
func pendingFromFile(path string) ([]pendingBook, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("retry-failed: %w", err)
	}
	defer func() { _ = file.Close() }()

	var books []pendingBook
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		md5, name, _ := strings.Cut(line, "|")
		md5 = strings.ToLower(strings.TrimSpace(md5))
		if !catalog.IsMD5(md5) {
			return nil, fmt.Errorf("retry-failed: linha inválida %q: md5 inválido", line)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = md5
		}
		books = append(books, pendingBook{
			md5:       md5,
			filename:  name,
			extension: strings.TrimPrefix(filepath.Ext(name), "."),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("retry-failed: %w", err)
	}
	return books, nil
}

// runRetryPasses walks the pending list once, then — under --loop — keeps
// retrying on the given pause until everything is downloaded or the context
// is cancelled.
func runRetryPasses(ctx context.Context, cfg config.Config, store *catalog.Store, chain *sources.Chain, pending []pendingBook, opts retryFailedOptions) error {
	for {
		remaining := retryPass(ctx, cfg, store, chain, pending, opts)
		if ctx.Err() != nil {
			return nil
		}
		if remaining == 0 {
			fmt.Println("TUDO BAIXADO")
			return nil
		}
		fmt.Printf("%d pendente(s)\n", remaining)
		if !opts.loop {
			return nil
		}
		fmt.Printf("nova passada em %s (ctrl-c para sair)\n", opts.pause)
		if err := waitOrDone(ctx, opts.pause); err != nil {
			return nil
		}
	}
}

// retryPass walks every pending book once and counts what is still missing.
// A book already on disk counts as done and clears its history row.
func retryPass(ctx context.Context, cfg config.Config, store *catalog.Store, chain *sources.Chain, pending []pendingBook, opts retryFailedOptions) int {
	remaining := 0
	for _, book := range pending {
		if ctx.Err() != nil {
			return remaining
		}
		dest := filepath.Join(opts.destDir, book.filename)
		if fileExists(dest) {
			markRetrySuccess(store, book)
			continue
		}
		usedAlt, result, err := retryOne(ctx, cfg, chain, book, dest, opts)
		if err != nil {
			remaining++
			fmt.Printf("pend %s: %s\n", book.md5, firstLine(err.Error()))
			continue
		}
		if usedAlt != "" {
			fmt.Printf("ok   %s (%s) via cópia alternativa %s\n", book.filename, humanBytes(result.Bytes), usedAlt[:8])
		} else {
			fmt.Printf("ok   %s (%s)\n", book.filename, humanBytes(result.Bytes))
		}
		markRetrySuccess(store, book)
	}
	return remaining
}

// retryOne downloads one book, pausing and retrying while the CDN reports the
// file as unsynced, then falling back to a synced copy of the same title when
// the exact md5 stays missing.
func retryOne(ctx context.Context, cfg config.Config, chain *sources.Chain, book pendingBook, dest string, opts retryFailedOptions) (string, libgen.Result, error) {
	attempt := func(ctx context.Context) (libgen.Result, error) {
		return chain.Download(ctx, libgen.DownloadRequest{
			MD5:       book.md5,
			Title:     book.title,
			Extension: book.extension,
			DestPath:  dest,
		})
	}
	result, err := retryUnsynced(ctx, retryOptions{
		attempts: opts.retries,
		interval: opts.retryEvery,
		notice: func(attempt int, pause time.Duration) {
			fmt.Printf("     %s ainda não sincronizou; nova tentativa em %s\n", book.md5[:8], pause)
		},
	}, attempt)
	if err == nil {
		return "", result, nil
	}
	if !opts.allowAlt || !errors.Is(err, libgen.ErrNotSynced) {
		return "", libgen.Result{}, err
	}
	return tryAlternate(ctx, cfg, chain, book, dest)
}

// tryAlternate searches the mirror by title and downloads the best-matching
// synced copy instead, so a file that never syncs on its node does not block
// the book forever. The replacement is verified against its own md5.
func tryAlternate(ctx context.Context, cfg config.Config, chain *sources.Chain, book pendingBook, dest string) (string, libgen.Result, error) {
	query := strings.TrimSpace(book.title)
	if query == "" {
		stem := strings.TrimSuffix(book.filename, filepath.Ext(book.filename))
		query = strings.ReplaceAll(stem, "-", " ")
	}
	if query == "" {
		return "", libgen.Result{}, libgen.ErrNotSynced
	}
	entries, err := cfg.Client().Search(ctx, query, 10)
	if err != nil {
		return "", libgen.Result{}, err
	}
	candidates := pickAlternates(entries, query, book.extension, map[string]bool{book.md5: true})
	if len(candidates) > alternateTries {
		candidates = candidates[:alternateTries]
	}
	for _, alternate := range candidates {
		md5 := strings.ToLower(strings.TrimSpace(alternate.MD5))
		fmt.Printf("     tentando cópia alternativa %s (%s)\n", md5[:8], alternate.Title)
		result, err := chain.Download(ctx, libgen.DownloadRequest{
			MD5:       md5,
			Title:     alternate.Title,
			Extension: alternate.Extension,
			DestPath:  dest,
		})
		if err == nil {
			return md5, result, nil
		}
	}
	return "", libgen.Result{}, libgen.ErrNotSynced
}

// markRetrySuccess flips the history row, so the page stops showing a book
// the reader now has. No-op for --from lists (no history row).
func markRetrySuccess(store *catalog.Store, book pendingBook) {
	if book.userID < 1 {
		return
	}
	_ = store.RecordDownload(catalog.DownloadRecord{
		UserID:    book.userID,
		MD5:       book.md5,
		Title:     book.title,
		Author:    book.author,
		Year:      book.year,
		Extension: book.extension,
		Filesize:  book.filesize,
		Failed:    false,
	})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// firstLine keeps pass output readable when the error is an errors.Join tree.
func firstLine(message string) string {
	line, _, found := strings.Cut(message, "\n")
	if !found {
		return line
	}
	return line + " …"
}

// destFilename derives a safe file name from a title, falling back to the
// md5 when the title is missing.
func destFilename(title, extension, md5 string) string {
	stem := slugifyName(title)
	if stem == "" {
		stem = md5
	}
	if ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), "."); ext != "" {
		return stem + "." + ext
	}
	return stem
}

func slugifyName(title string) string {
	var b strings.Builder
	dashed := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dashed = false
		case !dashed && b.Len() > 0:
			b.WriteByte('-')
			dashed = true
		}
	}
	return strings.Trim(b.String(), "-")
}
