package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// writeLibgenSnapshot builds a minimal stand-in for the libgen.sqlite3 dump:
// a books table with the mirror columns plus rows the importer must ignore.
func writeLibgenSnapshot(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "libgen.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	stmts := []string{
		`CREATE TABLE books (
			id INTEGER PRIMARY KEY,
			title TEXT, author TEXT, publisher TEXT, year INTEGER,
			language TEXT, extension TEXT, filesize INTEGER, md5 TEXT, searchable INTEGER
		)`,
		`INSERT INTO books (id, title, author, publisher, year, language, extension, filesize, md5, searchable)
		 VALUES (1, 'Harry Potter and the Philosopher''s Stone', 'J K Rowling', 'Bloomsbury', 1997, 'English', 'epub', 500000, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 1)`,
		`INSERT INTO books (id, title, author, publisher, year, language, extension, filesize, md5, searchable)
		 VALUES (2, 'The Pragmatic Programmer', 'Andrew Hunt', 'Addison-Wesley', 1999, 'English', 'pdf', 900000, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 1)`,
		`INSERT INTO books (id, title, author, publisher, year, language, extension, filesize, md5, searchable)
		 VALUES (3, '', '', '', 0, '', '', 0, '', 1)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture statement failed: %v", err)
		}
	}
	return path
}

func TestImportSQLite_LoadsBooksAndSearchable(t *testing.T) {
	store := newTestStore(t)
	snapshot := writeLibgenSnapshot(t)

	stats, err := store.ImportSQLite(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Imported != 2 {
		t.Errorf("Imported = %d, want 2 (empty titles are skipped)", stats.Imported)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}

	found, err := store.FindByRef("1")
	if err != nil {
		t.Fatal(err)
	}
	if found.Title != "Harry Potter and the Philosopher's Stone" || found.Author != "J K Rowling" {
		t.Errorf("imported book = %+v", found)
	}
	if found.Extension != "epub" || found.Year != 1997 || found.Filesize != 500000 {
		t.Errorf("imported metadata = %+v", found)
	}

	books, err := store.Search("pragmatic programmer", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].ID != 2 {
		t.Fatalf("Search after import = %+v", books)
	}
}

func TestImportSQLite_ReimportUpdatesInsteadOfDuplicating(t *testing.T) {
	store := newTestStore(t)
	snapshot := writeLibgenSnapshot(t)

	if _, err := store.ImportSQLite(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ImportSQLite(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM books`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("books after reimport = %d, want 2", count)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM books_fts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("books_fts after reimport = %d, want 2", count)
	}
}

func TestImportSQLite_MissingFile(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.ImportSQLite(context.Background(), filepath.Join(t.TempDir(), "nope.sqlite3")); err == nil {
		t.Fatal("ImportSQLite(missing) = nil error, want error")
	}
}
