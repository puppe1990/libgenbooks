package config

import "testing"

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
