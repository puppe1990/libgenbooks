// Package catalog owns the local Library Genesis book catalog: the SQLite
// schema (books / books_fts / cids), the Book projection and the lookups the
// CLI and the MCP tools share.
package catalog

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/migrate"
	caissqlite "github.com/puppe1990/amarra-cais/pkg/cais/sqlite"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Book is one edition of a work in the local libgen catalog.
type Book struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Year       int    `json:"year"`
	Extension  string `json:"extension"`
	Filesize   int64  `json:"filesize"`
	MD5        string `json:"md5"`
	Searchable int    `json:"searchable"`
}

// Store answers catalog lookups over the local SQLite libgen database.
type Store struct {
	db *sql.DB
}

// Open opens (creating when missing) the libgen catalog at dsn.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", caissqlite.DSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("open libgen catalog: %w", err)
	}
	if err := caissqlite.Configure(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure libgen catalog: %w", err)
	}
	if err := migrate.Apply(db, migrationsFS, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply libgen catalog migrations: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

// UpsertBook inserts or refreshes one edition.
func (s *Store) UpsertBook(b Book) error {
	return upsertBook(s.db, b)
}

// execer is *sql.DB or *sql.Tx, so the importer can reuse the upsert inside one
// transaction instead of paying a commit per book.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertBook(exec execer, b Book) error {
	_, err := exec.Exec(
		`INSERT INTO books (id, title, author, year, extension, filesize, md5, searchable)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title,
		   author = excluded.author,
		   year = excluded.year,
		   extension = excluded.extension,
		   filesize = excluded.filesize,
		   md5 = excluded.md5,
		   searchable = excluded.searchable`,
		b.ID, b.Title, b.Author, b.Year, b.Extension, b.Filesize, b.MD5, b.Searchable,
	)
	if err != nil {
		return fmt.Errorf("upsert libgen book %d: %w", b.ID, err)
	}
	// FTS5 tables reject UPSERT, so the row is replaced in two statements.
	if _, err := exec.Exec(`DELETE FROM books_fts WHERE rowid = ?`, b.ID); err != nil {
		return fmt.Errorf("delete libgen fts row %d: %w", b.ID, err)
	}
	if _, err := exec.Exec(
		`INSERT INTO books_fts (rowid, title, author, isbn) VALUES (?, ?, ?, ?)`,
		b.ID, b.Title, b.Author, b.MD5,
	); err != nil {
		return fmt.Errorf("upsert libgen fts row %d: %w", b.ID, err)
	}
	return nil
}

// FindByID returns the edition with the libgen catalog id.
func (s *Store) FindByID(id int64) (Book, error) {
	return scanBook(s.db.QueryRow(`SELECT id, title, author, year, extension, filesize, md5, searchable FROM books WHERE id = ?`, id))
}

// FindByMD5 returns the edition identified by its content hash.
func (s *Store) FindByMD5(md5 string) (Book, error) {
	return scanBook(s.db.QueryRow(`SELECT id, title, author, year, extension, filesize, md5, searchable FROM books WHERE md5 = ?`, md5))
}

// FindByRef resolves a user-facing reference: numeric libgen id, md5 hash or
// free-text query (resolved to the top hit). Errors when nothing matches.
func (s *Store) FindByRef(ref string) (Book, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Book{}, fmt.Errorf("empty book reference")
	}
	if isDigits(ref) {
		return s.FindByID(parseID(ref))
	}
	if len(ref) == 32 && allHex(ref) {
		return s.FindByMD5(strings.ToLower(ref))
	}
	books, err := s.Search(ref, 1)
	if err != nil {
		return Book{}, err
	}
	if len(books) == 0 {
		return Book{}, fmt.Errorf("no book found for %q", ref)
	}
	return books[0], nil
}

// Search runs an FTS5 full-text lookup across title, author and isbn and
// returns at most limit editions, best match first.
func (s *Store) Search(query string, limit int) ([]Book, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT b.id, b.title, b.author, b.year, b.extension, b.filesize, b.md5, b.searchable
		 FROM books_fts AS f
		 JOIN books AS b ON b.id = f.rowid
		 WHERE books_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		ftsQuery(query), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search libgen catalog: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var books []Book
	for rows.Next() {
		book, scanErr := scanBookRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate libgen search: %w", err)
	}
	return books, nil
}

func scanBookRow(rows *sql.Rows) (Book, error) {
	var b Book
	if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Year, &b.Extension, &b.Filesize, &b.MD5, &b.Searchable); err != nil {
		return Book{}, fmt.Errorf("scan libgen book: %w", err)
	}
	return b, nil
}

func scanBook(queryRow *sql.Row) (Book, error) {
	var b Book
	err := queryRow.Scan(&b.ID, &b.Title, &b.Author, &b.Year, &b.Extension, &b.Filesize, &b.MD5, &b.Searchable)
	if err != nil {
		return Book{}, fmt.Errorf("find libgen book: %w", err)
	}
	return b, nil
}

// ftsQuery turns a free-text query into a quoted FTS5 phrase OR chain. Tokens
// are stripped to alphanumerics before quoting, so the query cannot break the
// FTS syntax.
func ftsQuery(query string) string {
	var phrases []string
	tokens := splitTokens(query)
	seen := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		phrases = append(phrases, `"`+token+`"`)
	}
	if len(phrases) == 0 {
		return "''"
	}
	return strings.Join(phrases, " OR ")
}

func splitTokens(query string) []string {
	var tokens []string
	var current strings.Builder
	flushToken := func() {
		if current.Len() > 0 {
			tokens = append(tokens, strings.ToLower(current.String()))
			current.Reset()
		}
	}
	for _, r := range strings.ToLower(query) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
			continue
		}
		flushToken()
	}
	flushToken()
	return tokens
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseID(s string) int64 {
	var id int64
	_, _ = fmt.Sscanf(s, "%d", &id)
	return id
}

func allHex(s string) bool {
	for _, r := range strings.ToLower(s) {
		if !isHexDigit(r) {
			return false
		}
	}
	return true
}

// isHexDigit spells the class as a switch so the negated compound does not
// trip staticcheck's De Morgan rewrite.
func isHexDigit(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'f':
		return true
	default:
		return false
	}
}

// IsMD5 reports whether text looks like a libgen content hash.
func IsMD5(text string) bool {
	return len(text) == 32 && allHex(text)
}
