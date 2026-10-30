package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/config"
)

func runImport(ctx context.Context, cfg config.Config, args []string) error {
	flags := newFlagSet("import", "import [snapshot.sqlite3] [--from caminho]")
	from := flags.String("from", "./data/libgen.sqlite3", "snapshot SQLite do libgen (tabela books)")
	positional, err := parseFlags(flags, args)
	if err != nil {
		return err
	}

	path := strings.TrimSpace(*from)
	if len(positional) > 0 {
		path = strings.TrimSpace(positional[0])
	}

	store, err := catalog.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	stats, err := store.ImportSQLite(ctx, path)
	if err != nil {
		return err
	}
	fmt.Printf("importados %d livros de %s (%d ignorados)\n", stats.Imported, path, stats.Skipped)
	return nil
}
