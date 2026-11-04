package zlibrary

import (
	"strings"
	"unicode"
)

// TitleThreshold is the minimum word overlap between the wanted title and a
// search hit for it to count as the same book. Low enough for punctuation and
// casing variants ("Time-Tested" vs "Time Tested"), high enough to reject an
// unrelated record (a journal issue) whose text merely contains the phrase.
const TitleThreshold = 0.5

// minSharedWords is the overlap a multi-word title needs with a hit. A title
// that reduces to a single significant word ("Dune", "The Hobbit") falls back
// to one shared word — with nothing else to compare, demanding two made every
// one-word title unreachable. The cost is a one-word query accepting a
// compound hit ("walk" landing on "Walk-In"); without an author or year to
// tell them apart, rejecting the whole class was worse.
const minSharedWords = 2

// stopWords are the filler words dropped before comparing titles.
var stopWords = map[string]bool{
	"a": true, "an": true, "at": true, "and": true, "by": true, "for": true,
	"in": true, "of": true, "on": true, "the": true, "to": true, "with": true,
}

// accents folds the diacritics the word scanner would otherwise treat as
// separators: "Programação" must tokenize as one word, not "programa" + "o".
var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "ý", "y",
)

// TitleSimilarity scores two titles by the Jaccard overlap of their
// significant words: 1 when identical, 0 when they share nothing. The union
// denominator drops the score when a subtitle joins the hit, which is what
// ranks an exact copy ahead of its subtitled siblings.
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
// that share enough title words, best overlap first; a same-author copy gets
// a nudge that breaks ties without rescuing a title under the threshold.
func PickBook(books []Book, title, author, extension, wantedMD5 string) (Book, bool) {
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
	minWords := minShared(title)
	best := Book{}
	bestScore := 0.0
	for _, book := range books {
		if book.ID <= 0 || strings.TrimSpace(book.Hash) == "" {
			continue
		}
		if extension != "" && !strings.EqualFold(strings.TrimSpace(book.Extension), extension) {
			continue
		}
		if sharedWords(title, book.Title) < minWords {
			continue
		}
		score := TitleSimilarity(title, book.Title)
		if score < TitleThreshold && !containsWords(book.Title, title) && !containsWords(title, book.Title) {
			continue
		}
		if author != "" && sharedWords(author, book.Author) > 0 {
			score += 0.15
		}
		if score <= bestScore {
			continue
		}
		best, bestScore = book, score
	}
	return best, bestScore > 0
}

// minShared scales the overlap requirement with the wanted title.
func minShared(title string) int {
	if len(titleWords(title)) <= 1 {
		return 1
	}
	return minSharedWords
}

// containsWords reports whether every significant word of inner appears in
// outer. Either direction passing PickBook's filter means one title is the
// other's subtitle or a shorter edition of it ("Clean Code" against "Clean
// Code: A Handbook…"), which the union-based score alone would bury.
func containsWords(outer, inner string) bool {
	wordsOuter := titleWords(outer)
	wordsInner := titleWords(inner)
	if len(wordsInner) == 0 {
		return false
	}
	for word := range wordsInner {
		if !wordsOuter[word] {
			return false
		}
	}
	return true
}

// titleWords reduces a title to its significant lowercase words.
func titleWords(title string) map[string]bool {
	words := map[string]bool{}
	folded := accents.Replace(strings.ToLower(title))
	for _, word := range strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
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
