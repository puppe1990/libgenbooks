package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ImportStats reports what one snapshot import changed.
type ImportStats struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// ImportSQLite copies every row of a libgen SQLite snapshot (the libgen.sqlite3
// dump) into this catalog. Columns are matched by name, so snapshots carrying
// extra mirror columns still import.
func (s *Store) ImportSQLite(ctx context.Context, path string) (ImportStats, error) {
	if _, err := os.Stat(path); err != nil {
		return ImportStats{}, fmt.Errorf("open libgen snapshot %s: %w", path, err)
	}
	source, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return ImportStats{}, fmt.Errorf("open libgen snapshot %s: %w", path, err)
	}
	defer func() { _ = source.Close() }()

	rows, err := source.QueryContext(ctx, `SELECT * FROM books`)
	if err != nil {
		return ImportStats{}, fmt.Errorf("read books from %s: %w", path, err)
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return ImportStats{}, fmt.Errorf("read books columns from %s: %w", path, err)
	}
	index := snapshotColumnIndex(columns)
	if _, ok := index["id"]; !ok {
		return ImportStats{}, fmt.Errorf("import %s: books table has no id column (found %v)", path, columns)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportStats{}, fmt.Errorf("begin import of %s: %w", path, err)
	}
	defer func() { _ = tx.Rollback() }()

	stats, err := copySnapshotRows(rows, index, len(columns), func(book Book) error {
		return upsertBook(tx, book)
	})
	if err != nil {
		return ImportStats{}, fmt.Errorf("import %s: %w", path, err)
	}
	if err := tx.Commit(); err != nil {
		return ImportStats{}, fmt.Errorf("commit import of %s: %w", path, err)
	}
	return stats, nil
}

func copySnapshotRows(rows *sql.Rows, index map[string]int, width int, save func(Book) error) (ImportStats, error) {
	var stats ImportStats
	values := make([]any, width)
	targets := make([]any, width)
	for position := range values {
		targets[position] = &values[position]
	}

	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			return stats, fmt.Errorf("scan book row %d: %w", stats.Imported+stats.Skipped+1, err)
		}
		book := snapshotBook(values, index)
		if book.ID <= 0 || book.Title == "" {
			stats.Skipped++
			continue
		}
		if err := save(book); err != nil {
			return stats, err
		}
		stats.Imported++
	}
	if err := rows.Err(); err != nil {
		return stats, fmt.Errorf("iterate books: %w", err)
	}
	return stats, nil
}

func snapshotBook(values []any, index map[string]int) Book {
	book := Book{
		ID:        snapshotInt(values, index, "id"),
		Title:     strings.TrimSpace(snapshotText(values, index, "title")),
		Author:    strings.TrimSpace(snapshotText(values, index, "author")),
		Year:      int(snapshotInt(values, index, "year")),
		Extension: strings.TrimSpace(snapshotText(values, index, "extension")),
		Filesize:  snapshotInt(values, index, "filesize"),
		MD5:       strings.TrimSpace(snapshotText(values, index, "md5")),
	}
	if _, ok := index["searchable"]; ok {
		book.Searchable = int(snapshotInt(values, index, "searchable"))
	} else {
		book.Searchable = 1
	}
	return book
}

func snapshotColumnIndex(columns []string) map[string]int {
	index := make(map[string]int, len(columns))
	for position, name := range columns {
		index[strings.ToLower(strings.TrimSpace(name))] = position
	}
	return index
}

func snapshotText(values []any, index map[string]int, column string) string {
	position, ok := index[column]
	if !ok || position >= len(values) {
		return ""
	}
	return valueText(values[position])
}

func snapshotInt(values []any, index map[string]int, column string) int64 {
	text := strings.TrimSpace(snapshotText(values, index, column))
	if text == "" {
		return 0
	}
	if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(text, 64); err == nil {
		return int64(parsed)
	}
	return 0
}

func valueText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}
