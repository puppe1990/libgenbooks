package main

import (
	"sort"
	"strings"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/libgen"
	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

// alternateTitleThreshold is the minimum word overlap between the wanted
// title and a search hit for it to count as the same book. Low enough for
// punctuation and casing variants ("Time-Tested" vs "Time Tested"), high
// enough to reject an unrelated record (a journal issue) whose text merely
// contains the phrase.
const alternateTitleThreshold = 0.5

// alternateStopWords are the filler words dropped before comparing titles.
var alternateStopWords = map[string]bool{
	"a": true, "an": true, "at": true, "and": true, "by": true, "for": true,
	"in": true, "of": true, "on": true, "the": true, "to": true, "with": true,
}

// titleWords reduces a title to its significant lowercase words.
func titleWords(title string) map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if len(word) < 2 || alternateStopWords[word] {
			continue
		}
		words[word] = true
	}
	return words
}

// titleSimilarity scores two titles by the Jaccard overlap of their
// significant words: 1 when identical, 0 when they share nothing.
func titleSimilarity(a, b string) float64 {
	wordsA := titleWords(a)
	wordsB := titleWords(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}
	shared := 0
	for word := range wordsA {
		if wordsB[word] {
			shared++
		}
	}
	return float64(shared) / float64(len(wordsA)+len(wordsB)-shared)
}

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
		similarity := titleSimilarity(title, entry.Title)
		if similarity < alternateTitleThreshold {
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

// minZlibSharedWords keeps a one-word query from matching a one-word title:
// searching "walk" must not accept "Walk-In", while a full title still needs
// only two shared words to count.
const minZlibSharedWords = 2

// pickZlibBook ranks Z-Library search hits for a replacement copy of the
// wanted title: a hit whose md5 is the exact file that was asked for wins
// outright (Z-Library often carries the byte-identical libgen file), then
// same-extension copies with enough shared title words, best overlap first.
func pickZlibBook(books []zlibrary.Book, title, extension, wantedMD5 string) (zlibrary.Book, bool) {
	wanted := strings.ToLower(strings.TrimSpace(wantedMD5))
	if wanted != "" {
		for _, book := range books {
			if book.ID <= 0 || strings.TrimSpace(book.Hash) == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(book.MD5), wanted) {
				return book, true
			}
		}
	}
	best := zlibrary.Book{}
	bestScore := 0.0
	for _, book := range books {
		if book.ID <= 0 || strings.TrimSpace(book.Hash) == "" {
			continue
		}
		if extension != "" && !strings.EqualFold(strings.TrimSpace(book.Extension), extension) {
			continue
		}
		if sharedWords(title, book.Title) < minZlibSharedWords {
			continue
		}
		score := titleSimilarity(title, book.Title)
		if score < alternateTitleThreshold || score <= bestScore {
			continue
		}
		best, bestScore = book, score
	}
	return best, bestScore > 0
}

// sharedWords counts the significant words two titles have in common.
func sharedWords(a, b string) int {
	wordsA := titleWords(a)
	wordsB := titleWords(b)
	shared := 0
	for word := range wordsA {
		if wordsB[word] {
			shared++
		}
	}
	return shared
}
