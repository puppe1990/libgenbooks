// Package config reads the environment variables the CLI and the MCP server
// share, so both entry points talk to the same catalog file and mirror.
package config

import (
	"os"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/dotenv"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

// Config is the app-owned configuration (not the amarra-cais HTTP config).
type Config struct {
	DBPath      string
	BaseURL     string
	Cookie      string
	DownloadDir string
}

// Load reads LIBGEN_* variables, falling back to local defaults. A ./.env file
// is applied first when present so the CLI, the MCP server and `amarra-cais dev`
// agree on the same catalog; exported variables still win.
func Load() Config {
	_ = dotenv.LoadFile(".env")
	return Config{
		DBPath:      envOr("LIBGEN_DB", "./data/libgen.db"),
		BaseURL:     envOr("LIBGEN_BASE_URL", libgen.DefaultBaseURL),
		Cookie:      envOr("LIBGEN_COOKIE", ""),
		DownloadDir: envOr("LIBGEN_DOWNLOAD_DIR", "downloads"),
	}
}

// Client builds the mirror client with the configured session and download
// directory.
func (c Config) Client() *libgen.Client {
	client := libgen.New(c.BaseURL)
	client.Cookie = c.Cookie
	client.DownloadDir = c.DownloadDir
	return client
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
