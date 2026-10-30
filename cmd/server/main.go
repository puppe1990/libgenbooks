package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/boot"
	"github.com/puppe1990/amarra-cais/pkg/cais/devreload"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/puppe1990/libgenbooks/internal/app"
	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/store"
	"github.com/puppe1990/libgenbooks/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := cais.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	preferredPort := cfg.Port
	port, shifted, err := cais.ResolvePort(cfg.Port, cfg.Env)
	if err != nil {
		return err
	}
	cfg.Port = port

	// Graceful shutdown on SIGINT/SIGTERM (air sends SIGINT before each
	// rebuild when send_interrupt is set) so the listener, sqlite and the dev
	// watcher close instead of lingering as a zombie holding :8080 and
	// data/app.db (#77). The error is returned to main rather than exiting here
	// so this defer still runs (gocritic exitAfterDefer, #205).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := bootstrapWithConfig(ctx, cfg)
	if err != nil {
		return err
	}

	shiftedFrom := ""
	if shifted {
		shiftedFrom = preferredPort
	}
	boot.Print(os.Stdout, boot.Options{
		AppName:         "libgen-books",
		Config:          cfg,
		Version:         boot.CaisVersion(),
		PortShiftedFrom: shiftedFrom,
	})
	return a.RunContext(ctx)
}

func bootstrapWithConfig(ctx context.Context, cfg cais.Config) (*app.App, error) {
	catalog := appi18n.NewCatalog(cfg.Locale)
	catalogs := map[string]*i18n.Catalog{
		"en": appi18n.NewCatalog("en"),
		"pt": appi18n.NewCatalog("pt"),
	}

	views, err := loadViews(cfg, catalog, catalogs)
	if err != nil {
		return nil, fmt.Errorf("views: %w", err)
	}
	// Development re-parses templates from disk on edit and pings the open tabs
	// over SSE, so HTML/CSS changes need neither an air rebuild nor a page
	// restart (#340). Nil outside development.
	devReload := devreload.Watch(ctx, cfg, views.ReloadFromDir)

	s, err := store.NewSQLiteStore(cfg.DBPath, cfg.Env)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}

	staticDir, err := cais.ResolveWebDir("static", cfg.StaticDir)
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	return app.New(cfg, app.Deps{
		Views:     views,
		Store:     s,
		StaticDir: staticDir,
		Site:      meta.SiteFrom("libgen-books", cfg.AppURL),
		Catalog:   catalog,
		Catalogs:  catalogs,
		DevReload: devReload,
	})
}

func loadViews(cfg cais.Config, catalog *i18n.Catalog, catalogs map[string]*i18n.Catalog) (*view.Renderer, error) {
	tmplFS, err := templateFS(cfg)
	if err != nil {
		return nil, err
	}
	return view.Load(tmplFS, catalog, catalogs["en"], catalogs["pt"])
}

// templateFS reads web/templates from disk in development so the dev reload can
// swap the page set in-process; production keeps the embedded copy (#340).
func templateFS(cfg cais.Config) (fs.FS, error) {
	if cfg.Env != "development" {
		return fs.Sub(web.Templates, "templates")
	}
	dir, err := cais.ResolveWebDir("templates", cfg.TemplatesDir)
	if err != nil {
		return nil, err
	}
	info, statErr := os.Stat(dir)
	if statErr != nil || !info.IsDir() {
		// Running from another directory without TEMPLATES_DIR: the embedded
		// copy still boots, only the on-disk reload is unavailable.
		return fs.Sub(web.Templates, "templates")
	}
	return os.DirFS(dir), nil
}
