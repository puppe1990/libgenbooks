// Package config reads the environment variables the CLI and the MCP server
// share, so both entry points talk to the same catalog file and mirror.
package config

import (
	"os"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/dotenv"

	"github.com/puppe1990/libgenbooks/internal/annas"
	"github.com/puppe1990/libgenbooks/internal/ipfs"
	"github.com/puppe1990/libgenbooks/internal/libgen"
	"github.com/puppe1990/libgenbooks/internal/sources"
)

// Config is the app-owned configuration (not the amarra-cais HTTP config).
type Config struct {
	DBPath            string
	BaseURL           string
	Cookie            string
	DownloadDir       string
	HistoryOwnerEmail string
	// Sources is the LIBGEN_SOURCES provider chain, in preference order.
	Sources []string
	// AnnasURL overrides Anna's Archive's primary domain (LIBGEN_ANNAS_URL);
	// empty keeps the built-in .gl/.pk/.gd rotation.
	AnnasURL string
}

// Load reads LIBGEN_* variables, falling back to local defaults. A ./.env file
// is applied first when present so the CLI, the MCP server and `amarra-cais dev`
// agree on the same catalog; exported variables still win.
func Load() Config {
	_ = dotenv.LoadFile(".env")
	return Config{
		DBPath:            envOr("LIBGEN_DB", "./data/libgen.db"),
		BaseURL:           envOr("LIBGEN_BASE_URL", libgen.DefaultBaseURL),
		Cookie:            envOr("LIBGEN_COOKIE", ""),
		DownloadDir:       envOr("LIBGEN_DOWNLOAD_DIR", "downloads"),
		HistoryOwnerEmail: envOr("LIBGEN_HISTORY_OWNER_EMAIL", ""),
		Sources:           envList("LIBGEN_SOURCES", strings.Join(sources.DefaultNames, ",")),
		AnnasURL:          envOr("LIBGEN_ANNAS_URL", ""),
	}
}

// HistoryOwnerPassword reads the boot secret used to create the cutover
// account. Kept off Config so logs and boot banners never print it.
func HistoryOwnerPassword() string {
	return strings.TrimSpace(os.Getenv("LIBGEN_HISTORY_OWNER_PASSWORD"))
}

// Client builds the mirror client with the configured session and download
// directory.
func (c Config) Client() *libgen.Client {
	client := libgen.New(c.BaseURL)
	client.Cookie = c.Cookie
	client.DownloadDir = c.DownloadDir
	client.DownloadMirrors = libgen.DefaultDownloadMirrors
	return client
}

// Resolver builds the multi-source download chain (LibGen handshake first,
// Anna's Archive over IPFS gateways as fallback).
func (c Config) Resolver() (*sources.Chain, error) {
	client := c.Client()
	providers, err := sources.Providers(c.Sources, client, annas.New(c.AnnasURL), ipfs.New())
	if err != nil {
		return nil, err
	}
	return sources.New(client, providers), nil
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envList(key, fallback string) []string {
	parts := strings.Split(envOr(key, fallback), ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
