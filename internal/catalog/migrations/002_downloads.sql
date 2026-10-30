CREATE TABLE IF NOT EXISTS downloads (
    md5 TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    year INTEGER NOT NULL DEFAULT 0,
    extension TEXT NOT NULL DEFAULT '',
    filesize INTEGER NOT NULL DEFAULT 0,
    downloaded_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS downloads_downloaded_at ON downloads (downloaded_at DESC);
