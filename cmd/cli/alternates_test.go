package main

import (
	"strings"
	"testing"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func TestTitleSimilarity(t *testing.T) {
	cases := []struct {
		a, b        string
		wantAtLeast float64
		wantBelow   float64
	}{
		{
			a:           "Crisis Engineering Time-Tested Tools for Turning Chaos Into Clarity",
			b:           "Crisis Engineering Time Tested Tools For Turning Chaos Into Clarity",
			wantAtLeast: 0.9,
			wantBelow:   1.01,
		},
		{
			a:           "The Way of Excellence: A Guide to True Greatness",
			b:           "The Way of Excellence: A Guide to True Greatness and Deep Satisfaction",
			wantAtLeast: 0.6,
			wantBelow:   0.8,
		},
		{
			a:           "Crisis Engineering",
			b:           "2023-dec vol. 11 iss. 4",
			wantAtLeast: 0,
			wantBelow:   0.1,
		},
		{a: "How a Little Becomes a Lot", b: "", wantAtLeast: 0, wantBelow: 0.1},
	}
	for _, c := range cases {
		got := titleSimilarity(c.a, c.b)
		if got < c.wantAtLeast || got > c.wantBelow {
			t.Errorf("titleSimilarity(%q, %q) = %.2f, want [%.2f, %.2f]", c.a, c.b, got, c.wantAtLeast, c.wantBelow)
		}
	}
}

func TestPickAlternates_RejectsUnrelatedCopies(t *testing.T) {
	entries := []libgen.Entry{
		{MD5: strings.Repeat("a", 32), Title: "2023-dec vol. 11 iss. 4", Extension: "pdf"},
	}
	got := pickAlternates(entries, "Crisis Engineering Time-Tested Tools", "epub", map[string]bool{})
	if len(got) != 0 {
		t.Fatalf("alternates = %+v, want none: a journal issue merely mentioning the phrase", got)
	}
}

func TestPickAlternates_AcceptsAndRanksSameTitleCopies(t *testing.T) {
	near := libgen.Entry{MD5: strings.Repeat("b", 32), Title: "The Way of Excellence: A Guide to True Greatness and Deep Satisfaction in a Chaotic World", Extension: "epub"}
	far := libgen.Entry{MD5: strings.Repeat("c", 32), Title: "The Way of Excellence: A Guide to True Greatness", Extension: "epub"}
	other := libgen.Entry{MD5: strings.Repeat("d", 32), Title: "The Way of Excellence: A Guide to True Greatness and Deep Satisfaction in a Chaotic World", Extension: "pdf"}
	tried := map[string]bool{strings.Repeat("e", 32): true}
	entries := []libgen.Entry{far, other, near, {MD5: strings.Repeat("e", 32), Title: "The Way of Excellence: A Guide to True Greatness and Deep Satisfaction in a Chaotic World", Extension: "epub"}}

	got := pickAlternates(entries, "The Way of Excellence: A Guide to True Greatness and Deep Satisfaction in a Chaotic World", "epub", tried)
	if len(got) != 2 {
		t.Fatalf("alternates = %+v, want the two matching epub copies", got)
	}
	if got[0].MD5 != near.MD5 || got[1].MD5 != far.MD5 {
		t.Fatalf("order = %s then %s, want the closer title first", got[0].MD5[:4], got[1].MD5[:4])
	}
}

func TestPickAlternates_AllowsAnyExtensionWhenWantedIsUnknown(t *testing.T) {
	entries := []libgen.Entry{
		{MD5: strings.Repeat("f", 32), Title: "Open to Work How to Get Ahead in the Age of AI", Extension: "pdf"},
	}
	got := pickAlternates(entries, "Open to Work How to Get Ahead in the Age of AI", "", map[string]bool{})
	if len(got) != 1 {
		t.Fatalf("alternates = %+v, want the pdf copy when no extension was requested", got)
	}
}
