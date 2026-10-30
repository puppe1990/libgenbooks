package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStore_UpsertThenFindByID(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBook(Book{ID: 7, Title: "Harry Potter and the Sorcerer's Stone", Author: "J K Rowling", Year: 1997, Extension: "mobi", Filesize: 600000, MD5: "abc123", Searchable: 1}); err != nil {
		t.Fatal(err)
	}
	found, err := s.FindByID(7)
	if err != nil {
		t.Fatal(err)
	}
	if found.Title != "Harry Potter and the Sorcerer's Stone" || found.Author != "J K Rowling" {
		t.Fatalf("found book = %+v", found)
	}
	if found.MD5 != "abc123" || found.Filesize != 600000 {
		t.Fatalf("found book scalars = %+v", found)
	}
}

func TestStore_FindByMD5(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBook(Book{ID: 9, Title: "Dune", Author: "Frank Herbert", MD5: "deadbeef", Searchable: 1}); err != nil {
		t.Fatal(err)
	}
	found, err := s.FindByMD5("deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != 9 || found.Title != "Dune" {
		t.Fatalf("found book = %+v", found)
	}
}

func TestStore_FindByRef_Md5AndId(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBook(Book{ID: 3, Title: "Clean Code", Author: "Robert Martin", MD5: "cafebabe", Searchable: 1}); err != nil {
		t.Fatal(err)
	}

	if found, err := s.FindByRef("3"); err != nil || found.Title != "Clean Code" {
		t.Fatalf("FindByRef(id) = %+v, %v", found, err)
	}
	if found, err := s.FindByRef("cafebabe"); err != nil || found.ID != 3 {
		t.Fatalf("FindByRef(md5) = %+v, %v", found, err)
	}
	if _, err := s.FindByRef("livro inexistente"); err == nil {
		t.Fatal("FindByRef(unknown) = nil error, want error")
	}
}

func TestStore_Search_ByTitle(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBook(Book{ID: 1, Title: "Harry Potter and the Philosopher's Stone", Author: "J K Rowling", MD5: "h1", Searchable: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertBook(Book{ID: 2, Title: "The Pragmatic Programmer", Author: "Andrew Hunt", MD5: "h2", Searchable: 1}); err != nil {
		t.Fatal(err)
	}

	books, err := s.Search("harry potter", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].ID != 1 {
		t.Fatalf("Search(harry potter) = %+v, want one book with id 1", books)
	}
}

func TestStore_Search_ByAuthor(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBook(Book{ID: 5, Title: "The Silmarillion", Author: "J R R Tolkien", MD5: "h5", Searchable: 1}); err != nil {
		t.Fatal(err)
	}

	books, err := s.Search("tolkien", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].ID != 5 {
		t.Fatalf("Search(tolkien) = %+v, want one book with id 5", books)
	}
}

func TestStore_Search_OrdersNewestYearFirst(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBook(Book{ID: 1, Title: "Clean Code", Author: "Robert Martin", Year: 2008, MD5: "old", Searchable: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertBook(Book{ID: 2, Title: "Clean Code", Author: "Robert Martin", Year: 2024, MD5: "new", Searchable: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertBook(Book{ID: 3, Title: "Clean Code", Author: "Someone", Year: 0, MD5: "none", Searchable: 1}); err != nil {
		t.Fatal(err)
	}

	books, err := s.Search("clean code", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 3 {
		t.Fatalf("Search = %d books, want 3", len(books))
	}
	if books[0].Year != 2024 || books[0].ID != 2 {
		t.Fatalf("first = %+v, want year 2024", books[0])
	}
	if books[1].Year != 2008 {
		t.Fatalf("second = %+v, want year 2008", books[1])
	}
	if books[2].Year != 0 {
		t.Fatalf("last = %+v, want missing year last", books[2])
	}
}

func TestStore_Search_UnknownTerm(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBook(Book{ID: 5, Title: "The Silmarillion", Author: "J R R Tolkien", MD5: "h5", Searchable: 1}); err != nil {
		t.Fatal(err)
	}
	books, err := s.Search("xyzzy unknown", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 0 {
		t.Fatalf("Search(unknown) = %+v, want empty", books)
	}
}

func TestOpen_CreatesMissingParentDirectory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data", "nested", "libgen.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%s) = %v, want the parent directory to be created", dbPath, err)
	}
	defer func() { _ = s.Close() }()

	if err := s.UpsertBook(Book{ID: 1, Title: "Dune", Searchable: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("catalog file %s missing: %v", dbPath, err)
	}
}
