package zlibrary

import "strings"

// TitleThreshold is the minimum word overlap between the wanted title and a
// search hit for it to count as the same book. Low enough for punctuation and
// casing variants ("Time-Tested" vs "Time Tested"), high enough to reject an
// unrelated record (a journal issue) whose text merely contains the phrase.
const TitleThreshold = 0.5

// minSharedWords keeps a one-word query from matching a one-word title:
// searching "walk" must not accept "Walk-In", while a full title still needs
// only two shared words to count.
const minSharedWords = 2

// stopWords are the filler words dropped before comparing titles.
var stopWords = map[string]bool{
	"a": true, "an": true, "at": true, "and": true, "by": true, "for": true,
	"in": true, "of": true, "on": true, "the": true, "to": true, "with": true,
}

// TitleSimilarity scores two titles by the Jaccard overlap of their
// significant words: 1 when identical, 0 when they share nothing.
func TitleSimilarity(a, b string) float64 {
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

// PickBook ranks search hits for a replacement copy of the wanted title: a
// hit whose md5 is the exact file that was asked for wins outright (Z-Library
// often carries the byte-identical libgen file), then same-extension copies
// with enough shared title words, best overlap first.
func PickBook(books []Book, title, extension, wantedMD5 string) (Book, bool) {
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
	best := Book{}
	bestScore := 0.0
	for _, book := range books {
		if book.ID <= 0 || strings.TrimSpace(book.Hash) == "" {
			continue
		}
		if extension != "" && !strings.EqualFold(strings.TrimSpace(book.Extension), extension) {
			continue
		}
		if sharedWords(title, book.Title) < minSharedWords {
			continue
		}
		score := TitleSimilarity(title, book.Title)
		if score < TitleThreshold || score <= bestScore {
			continue
		}
		best, bestScore = book, score
	}
	return best, bestScore > 0
}

// titleWords reduces a title to its significant lowercase words.
func titleWords(title string) map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if len(word) < 2 || stopWords[word] {
			continue
		}
		words[word] = true
	}
	return words
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
