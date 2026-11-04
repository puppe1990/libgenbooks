package zlibrary

import "testing"

func TestTitleWords_FoldsAccentsInsteadOfSlicing(t *testing.T) {
	words := titleWords("Introdução à Programação")
	if len(words) != 2 || !words["introducao"] || !words["programacao"] {
		t.Fatalf("words = %v, want introducao and programacao as whole words", words)
	}
}

func TestTitleSimilarity_FoldsBothSidesOfTheAccent(t *testing.T) {
	if got := TitleSimilarity("Introdução à Programação", "Introducao a Programacao"); got != 1 {
		t.Fatalf("similarity = %.2f, want 1: accents must not split the words", got)
	}
}

func TestPickBook_SingleWordTitleReachesItsCopies(t *testing.T) {
	books := []Book{
		{ID: 1, Hash: "aaa111", Title: "Dune: A Novel", Extension: "epub", MD5: "11111111111111111111111111111111"},
		{ID: 2, Hash: "bbb222", Title: "Foundation", Extension: "epub"},
	}
	got, ok := PickBook(books, "Dune", "", "epub", "")
	if !ok || got.ID != 1 {
		t.Fatalf("PickBook = %+v ok=%v, want the Dune copy", got, ok)
	}
}

func TestPickBook_SubtitleDoesNotHideTheCopy(t *testing.T) {
	books := []Book{
		{ID: 3, Hash: "ccc333", Title: "Clean Code: A Handbook of Agile Software Craftsmanship", Extension: "pdf"},
	}
	got, ok := PickBook(books, "Clean Code", "", "pdf", "")
	if !ok || got.ID != 3 {
		t.Fatalf("PickBook = %+v ok=%v, want the subtitled copy", got, ok)
	}
}

func TestPickBook_TwoWordTitleRejectsOneWordOverlap(t *testing.T) {
	books := []Book{
		{ID: 4, Hash: "ddd444", Title: "Code Complete", Extension: "epub"},
	}
	if got, ok := PickBook(books, "Clean Code", "", "epub", ""); ok {
		t.Fatalf("PickBook = %+v, want none: one shared word of a two-word title is not a match", got)
	}
}

func TestPickBook_AuthorBreaksTies(t *testing.T) {
	books := []Book{
		{ID: 5, Hash: "eee555", Title: "Python Crash Course", Author: "Someone Else", Extension: "epub"},
		{ID: 6, Hash: "fff666", Title: "Python Crash Course", Author: "Eric Matthes", Extension: "epub"},
	}
	got, ok := PickBook(books, "Python Crash Course", "Eric Matthes", "epub", "")
	if !ok || got.ID != 6 {
		t.Fatalf("PickBook = %+v ok=%v, want the same-author copy ahead of the tie", got, ok)
	}
}

func TestPickBook_AuthorBoostDoesNotRescueBelowThreshold(t *testing.T) {
	books := []Book{
		{ID: 7, Hash: "ggg777", Title: "Pragmatic Programming Notes for the Working Journeyman", Author: "Andrew Hunt", Extension: "epub"},
	}
	if got, ok := PickBook(books, "The Pragmatic Programmer from Journeyman to Master", "Andrew Hunt", "epub", ""); ok {
		t.Fatalf("PickBook = %+v, want none: the author must not rescue a weak title", got)
	}
}
