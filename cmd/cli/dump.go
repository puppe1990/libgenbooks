package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/puppe1990/libgenbooks/internal/config"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func runDump(ctx context.Context, cfg config.Config, args []string) error {
	flags := newFlagSet("dump", "dump [--list] [--part N] [--out caminho] [--url URL]")
	list := flags.Bool("list", false, "listar os dumps publicados pelo espelho")
	part := flags.Int("part", 1, "número da parte do dump principal (1..N)")
	out := flags.String("out", "", "arquivo de destino")
	directURL := flags.String("url", "", "URL exata a baixar")
	if _, err := parseFlags(flags, args); err != nil {
		return err
	}

	client := cfg.Client()
	if *directURL != "" {
		return downloadDump(ctx, cfg, *directURL, *out)
	}

	dumps, err := client.Dumps(ctx)
	if err != nil {
		return err
	}
	if *list {
		printDumpList(dumps)
		return nil
	}

	parts := libgen.MainDumpParts(dumps)
	if len(parts) == 0 {
		return fmt.Errorf("dump: nenhum conjunto libgen_new-<data>.partNNN.rar em %s", cfg.BaseURL)
	}
	if *part < 1 || *part > len(parts) {
		return fmt.Errorf("dump: --part %d fora do intervalo 1..%d", *part, len(parts))
	}
	target := parts[*part-1]
	destination := *out
	if destination == "" {
		destination = filepath.Join("data", "dumps", target.Name)
	}
	return downloadDump(ctx, cfg, target.URL, destination)
}

func downloadDump(ctx context.Context, cfg config.Config, rawURL, destination string) error {
	if destination == "" {
		destination = filepath.Join("data", "dumps", filepath.Base(rawURL))
	}
	fmt.Printf("baixando %s …\n", rawURL)
	result, err := cfg.Client().FetchFile(ctx, rawURL, destination)
	if err != nil {
		return err
	}
	fmt.Printf("baixado: %s (%s)\n", result.Path, humanBytes(result.Bytes))
	return nil
}

func printDumpList(dumps []libgen.DumpFile) {
	for _, dump := range dumps {
		fmt.Printf("%-45s %10s  %s\n", dump.Name, humanBytes(dump.Size), dump.Modified)
	}
}
