package config

import (
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/fakedata"
)

func TestLoad_Defaults(t *testing.T) {
	for _, key := range []string{"LIBGEN_DB", "LIBGEN_BASE_URL", "LIBGEN_COOKIE", "LIBGEN_DOWNLOAD_DIR", "LIBGEN_HISTORY_OWNER_EMAIL", "LIBGEN_HISTORY_OWNER_PASSWORD"} {
		t.Setenv(key, "")
	}

	cfg := Load()
	if cfg.DBPath != "./data/libgen.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.BaseURL != "https://libgen.li" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.DownloadDir != "downloads" {
		t.Errorf("DownloadDir = %q", cfg.DownloadDir)
	}
	if cfg.Cookie != "" {
		t.Errorf("Cookie = %q, want empty", cfg.Cookie)
	}
	if cfg.HistoryOwnerEmail != "" {
		t.Errorf("HistoryOwnerEmail = %q, want empty", cfg.HistoryOwnerEmail)
	}
	if HistoryOwnerPassword() != "" {
		t.Errorf("HistoryOwnerPassword = %q, want empty", HistoryOwnerPassword())
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("LIBGEN_DB", "/tmp/custom.db")
	t.Setenv("LIBGEN_BASE_URL", "https://libgen.ad")
	t.Setenv("LIBGEN_COOKIE", "lg_session=abc")
	t.Setenv("LIBGEN_DOWNLOAD_DIR", "/tmp/books")
	t.Setenv("LIBGEN_HISTORY_OWNER_EMAIL", "matheus.puppe@gmail.com")
	t.Setenv("LIBGEN_HISTORY_OWNER_PASSWORD", "secret-from-env")

	cfg := Load()
	if cfg.DBPath != "/tmp/custom.db" || cfg.BaseURL != "https://libgen.ad" {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.Cookie != "lg_session=abc" || cfg.DownloadDir != "/tmp/books" {
		t.Errorf("cfg = %+v", cfg)
	}

	client := cfg.Client()
	if client.BaseURL != "https://libgen.ad" || client.Cookie != "lg_session=abc" || client.DownloadDir != "/tmp/books" {
		t.Errorf("client = %+v", client)
	}
	if len(client.DownloadMirrors) == 0 {
		t.Fatal("DownloadMirrors empty, want sibling libgen hosts for ads.php failover")
	}
	if cfg.HistoryOwnerEmail != "matheus.puppe@gmail.com" {
		t.Errorf("HistoryOwnerEmail = %q", cfg.HistoryOwnerEmail)
	}
	if HistoryOwnerPassword() != "secret-from-env" {
		t.Errorf("HistoryOwnerPassword = %q", HistoryOwnerPassword())
	}
}

func TestLoad_SourcesDefault(t *testing.T) {
	t.Setenv("LIBGEN_SOURCES", "")
	t.Setenv("LIBGEN_ANNAS_URL", "")

	cfg := Load()
	if got := strings.Join(cfg.Sources, ","); got != "libgen,annas" {
		t.Fatalf("Sources = %v, want the libgen,annas default", cfg.Sources)
	}
	if cfg.AnnasURL != "" {
		t.Fatalf("AnnasURL = %q, want empty (built-in domain rotation)", cfg.AnnasURL)
	}
}

func TestLoad_SourcesOverride(t *testing.T) {
	t.Setenv("LIBGEN_SOURCES", " libgen , ")

	cfg := Load()
	if got := strings.Join(cfg.Sources, ","); got != "libgen" {
		t.Fatalf("Sources = %v, want [libgen]", cfg.Sources)
	}
}

func TestResolver_UnknownSourceFails(t *testing.T) {
	cfg := Config{Sources: []string{"megaupload"}}
	if _, err := cfg.Resolver(); err == nil || !strings.Contains(err.Error(), "megaupload") {
		t.Fatalf("err = %v, want the unknown provider named", err)
	}
}

func TestResolver_LibgenOnlyChain(t *testing.T) {
	cfg := Config{Sources: []string{"libgen"}, BaseURL: "https://libgen.invalid"}
	if _, err := cfg.Resolver(); err != nil {
		t.Fatalf("Resolver() = %v, want a libgen-only chain", err)
	}
}

func TestLoad_ZlibCredentials(t *testing.T) {
	fakedata.Seed(42)
	email := fakedata.Email()
	password := fakedata.Sentence()
	apiURL := strings.TrimRight(fakedata.URL(), "/")
	t.Setenv("LIBGEN_ZLIB_EMAIL", email)
	t.Setenv("LIBGEN_ZLIB_PASSWORD", password)
	t.Setenv("LIBGEN_ZLIB_URL", apiURL)

	cfg := Load()
	if !cfg.ZlibraryEnabled() {
		t.Fatal("ZlibraryEnabled() = false with credentials set")
	}
	client := cfg.Zlibrary()
	if client.Email != email || client.Password != password || client.BaseURL != apiURL {
		t.Fatalf("client = %+v", client)
	}
}

func TestLoad_ZlibDisabledWithoutCredentials(t *testing.T) {
	t.Setenv("LIBGEN_ZLIB_EMAIL", "")
	t.Setenv("LIBGEN_ZLIB_PASSWORD", "")
	t.Setenv("LIBGEN_ZLIB_URL", "")

	cfg := Load()
	if cfg.ZlibraryEnabled() {
		t.Fatal("ZlibraryEnabled() = true without credentials")
	}
	if cfg.Zlibrary().BaseURL == "" {
		t.Fatal("Zlibrary() must fall back to the default API host")
	}
}
