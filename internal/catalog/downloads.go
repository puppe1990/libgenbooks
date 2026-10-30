package catalog

import (
	"fmt"
	"strings"
	"time"
)

const defaultDownloadHistoryLimit = 100
const maxDownloadHistoryLimit = 500

// DownloadRecord is one click on Baixar, keyed by md5 so a second click
// bumps the timestamp instead of duplicating the row.
type DownloadRecord struct {
	UserID       int64
	MD5          string
	Title        string
	Author       string
	Year         int
	Extension    string
	Filesize     int64
	DownloadedAt time.Time
	Failed       bool
}

// RecordDownload inserts or refreshes a history row for this user and md5.
func (s *Store) RecordDownload(rec DownloadRecord) error {
	if rec.UserID < 1 {
		return fmt.Errorf("record download: missing user id")
	}
	md5 := strings.ToLower(strings.TrimSpace(rec.MD5))
	if !IsMD5(md5) {
		return fmt.Errorf("record download: invalid md5 %q: want 32 hex characters", rec.MD5)
	}
	when := rec.DownloadedAt.UTC()
	if when.IsZero() {
		when = time.Now().UTC()
	}
	failed := 0
	if rec.Failed {
		failed = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO downloads (user_id, md5, title, author, year, extension, filesize, downloaded_at, failed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, md5) DO UPDATE SET
		   title = CASE WHEN excluded.title = '' THEN downloads.title ELSE excluded.title END,
		   author = CASE WHEN excluded.author = '' THEN downloads.author ELSE excluded.author END,
		   year = CASE WHEN excluded.year = 0 THEN downloads.year ELSE excluded.year END,
		   extension = CASE WHEN excluded.extension = '' THEN downloads.extension ELSE excluded.extension END,
		   filesize = CASE WHEN excluded.filesize = 0 THEN downloads.filesize ELSE excluded.filesize END,
		   downloaded_at = excluded.downloaded_at,
		   failed = excluded.failed`,
		rec.UserID, md5, rec.Title, rec.Author, rec.Year, rec.Extension, rec.Filesize, when.Format(time.RFC3339Nano), failed,
	)
	if err != nil {
		return fmt.Errorf("record download %s: %w", md5, err)
	}
	return nil
}

// ListDownloads returns this user's history newest-first, at most limit rows.
func (s *Store) ListDownloads(userID int64, limit int) ([]DownloadRecord, error) {
	return s.queryDownloads(userID, limit, false)
}

// ListFailedDownloads returns this user's failed rows, newest-first.
func (s *Store) ListFailedDownloads(userID int64, limit int) ([]DownloadRecord, error) {
	return s.queryDownloads(userID, limit, true)
}

// ClaimOrphanDownloads assigns leftover user_id=0 rows (pre-tenant history)
// to userID so a cutover owner keeps the anonymous clicks.
func (s *Store) ClaimOrphanDownloads(userID int64) error {
	if userID < 1 {
		return fmt.Errorf("claim orphan downloads: missing user id")
	}
	_, err := s.db.Exec(`UPDATE downloads SET user_id = ? WHERE user_id = 0`, userID)
	if err != nil {
		return fmt.Errorf("claim orphan downloads for user %d: %w", userID, err)
	}
	return nil
}

func (s *Store) queryDownloads(userID int64, limit int, failedOnly bool) ([]DownloadRecord, error) {
	if limit <= 0 {
		limit = defaultDownloadHistoryLimit
	}
	if limit > maxDownloadHistoryLimit {
		limit = maxDownloadHistoryLimit
	}
	query := `SELECT user_id, md5, title, author, year, extension, filesize, downloaded_at, failed
		 FROM downloads WHERE user_id = ?`
	if failedOnly {
		query += ` AND failed = 1`
	}
	query += ` ORDER BY downloaded_at DESC LIMIT ?`
	rows, err := s.db.Query(query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list downloads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []DownloadRecord
	for rows.Next() {
		rec, scanErr := scanDownloadRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate downloads: %w", err)
	}
	return records, nil
}

func scanDownloadRow(rows interface {
	Scan(dest ...any) error
}) (DownloadRecord, error) {
	var rec DownloadRecord
	var when string
	var failed int
	if err := rows.Scan(&rec.UserID, &rec.MD5, &rec.Title, &rec.Author, &rec.Year, &rec.Extension, &rec.Filesize, &when, &failed); err != nil {
		return DownloadRecord{}, fmt.Errorf("scan download: %w", err)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, when); err == nil {
		rec.DownloadedAt = parsed.UTC()
	}
	rec.Failed = failed != 0
	return rec, nil
}
