package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/puppe1990/libgenbooks/internal/config"
)

const usage = `libgen-books — busca e baixa livros do Library Genesis

uso:
  libgen-books search <título|autor|isbn> [--limit N] [--local]
  libgen-books download <id|md5|título> [--out caminho] [--url URL]
  libgen-books import [snapshot.sqlite3] [--from caminho]
  libgen-books dump [--list] [--part N] [--out caminho] [--url URL]
  libgen-books help

variáveis:
  LIBGEN_DB            catálogo SQLite local (padrão ./data/libgen.db)
  LIBGEN_BASE_URL      espelho (padrão https://libgen.li)
  LIBGEN_COOKIE        cookie de sessão para record pages que exigem login
  LIBGEN_DOWNLOAD_DIR  pasta dos downloads (padrão downloads)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "libgen-books: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	switch args[0] {
	case "search", "s":
		return runSearch(ctx, cfg, args[1:])
	case "download", "d":
		return runDownload(ctx, cfg, args[1:])
	case "import", "i":
		return runImport(ctx, cfg, args[1:])
	case "dump":
		return runDump(ctx, cfg, args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q (run `libgen-books help`)", args[0])
	}
}

func newFlagSet(name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "uso: libgen-books %s\n", synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags accepts flags before or after the positional arguments: the
// standard flag package stops at the first non-flag, and `search clean code
// --limit 5` is the natural way to type it.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		remaining := fs.Args()
		if len(remaining) == 0 {
			return positional, nil
		}
		positional = append(positional, remaining[0])
		args = remaining[1:]
	}
}
