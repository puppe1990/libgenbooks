CREATE TABLE downloads_by_user (
    user_id INTEGER NOT NULL,
    md5 TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    year INTEGER NOT NULL DEFAULT 0,
    extension TEXT NOT NULL DEFAULT '',
    filesize INTEGER NOT NULL DEFAULT 0,
    downloaded_at TEXT NOT NULL,
    failed INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, md5)
);

INSERT INTO downloads_by_user (user_id, md5, title, author, year, extension, filesize, downloaded_at, failed)
SELECT 0, md5, title, author, year, extension, filesize, downloaded_at, failed
FROM downloads;

DROP TABLE downloads;

ALTER TABLE downloads_by_user RENAME TO downloads;

CREATE INDEX IF NOT EXISTS downloads_user_downloaded_at ON downloads (user_id, downloaded_at DESC);
