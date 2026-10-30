package cache

import (
	"testing"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func TestSaveEntries_MakesThemSearchableLocally(t *testing.T) {
	store, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	saved := SaveEntries(store, []libgen.Entry{
		{ID: 10, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "Clean Code", Author: "Robert Martin", Year: 2008, Extension: "pdf", Filesize: 1000},
		{ID: 11, Title: ""},
	})
	if saved != 1 {
		t.Fatalf("saved = %d, want 1 (entries without title are skipped)", saved)
	}

	books, err := store.Search("clean code", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].ID != 10 {
		t.Fatalf("local search = %+v", books)
	}
	if books[0].MD5 != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || books[0].Author != "Robert Martin" {
		t.Errorf("cached book = %+v", books[0])
	}
}
