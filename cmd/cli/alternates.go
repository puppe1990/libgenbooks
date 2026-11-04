package main

import (
	"sort"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

// pickAlternates ranks mirror hits as replacement copies for an md5 whose
// file never synchronized: same extension (any when the wanted one is
// unknown), enough title overlap, not already tried, best match first.
func pickAlternates(entries []libgen.Entry, title, extension string, tried map[string]bool) []libgen.Entry {
	type scored struct {
		entry      libgen.Entry
		similarity float64
	}
	var picked []scored
	for _, entry := range entries {
		md5 := strings.ToLower(strings.TrimSpace(entry.MD5))
		if !catalog.IsMD5(md5) || tried[md5] {
			continue
		}
		if entry.Title == "" {
			continue
		}
		if extension != "" && !strings.EqualFold(strings.TrimSpace(entry.Extension), extension) {
			continue
		}
		similarity := zlibrary.TitleSimilarity(title, entry.Title)
		if similarity < zlibrary.TitleThreshold {
			continue
		}
		picked = append(picked, scored{entry: entry, similarity: similarity})
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].similarity > picked[j].similarity })

	alternates := make([]libgen.Entry, 0, len(picked))
	for _, candidate := range picked {
		alternates = append(alternates, candidate.entry)
	}
	return alternates
}

// pickZlibBook ranks Z-Library search hits; the shared ranking lives in the
// API package so the web handler can use the same rules.
func pickZlibBook(books []zlibrary.Book, title, author, extension, wantedMD5 string) (zlibrary.Book, bool) {
	return zlibrary.PickBook(books, title, author, extension, wantedMD5)
}
