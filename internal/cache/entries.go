// Package cache keeps live mirror hits in the local catalog, so a later
// `download <title>` and the offline search still resolve after the session
// that found them.
package cache

import (
	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
)

// SaveEntries upserts the searchable columns of each live hit and returns how
// many rows were written.
func SaveEntries(store *catalog.Store, entries []libgen.Entry) int {
	saved := 0
	for _, entry := range entries {
		if entry.ID <= 0 || entry.Title == "" {
			continue
		}
		err := store.UpsertBook(catalog.Book{
			ID:         entry.ID,
			Title:      entry.Title,
			Author:     entry.Author,
			Year:       entry.Year,
			Extension:  entry.Extension,
			Filesize:   entry.Filesize,
			MD5:        entry.MD5,
			Searchable: 1,
		})
		if err == nil {
			saved++
		}
	}
	return saved
}
