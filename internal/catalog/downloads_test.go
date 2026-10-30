package catalog

import (
	"testing"
	"time"
)

func TestRecordDownload_ListsNewestFirst(t *testing.T) {
	s := newTestStore(t)
	older := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	if err := s.RecordDownload(DownloadRecord{
		MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "Older", DownloadedAt: older,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{
		MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Title: "Newer", Author: "Ada", Year: 2024, Extension: "epub", DownloadedAt: newer,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDownloads(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "Newer" || got[0].Author != "Ada" || got[0].Extension != "epub" {
		t.Fatalf("first = %+v, want the newer download", got[0])
	}
	if got[1].Title != "Older" {
		t.Fatalf("second = %+v, want Older", got[1])
	}
}

func TestRecordDownload_UpsertsSameMD5(t *testing.T) {
	s := newTestStore(t)
	md5 := "cccccccccccccccccccccccccccccccc"
	first := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := s.RecordDownload(DownloadRecord{MD5: md5, Title: "First", DownloadedAt: first}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{MD5: md5, Title: "Second", Author: "Bob", DownloadedAt: first.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDownloads(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (upsert by md5)", len(got))
	}
	if got[0].Title != "Second" || got[0].Author != "Bob" {
		t.Fatalf("got = %+v, want updated title and author", got[0])
	}
}

func TestRecordDownload_RejectsBadMD5(t *testing.T) {
	s := newTestStore(t)
	if err := s.RecordDownload(DownloadRecord{MD5: "nope", Title: "X"}); err == nil {
		t.Fatal("RecordDownload(nope) = nil, want error")
	}
}

func TestListDownloads_Empty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.ListDownloads(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestRecordDownload_StoresFailed(t *testing.T) {
	s := newTestStore(t)
	md5 := "dddddddddddddddddddddddddddddddd"
	if err := s.RecordDownload(DownloadRecord{MD5: md5, Title: "Crisis", Failed: true}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDownloads(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Failed || got[0].Title != "Crisis" {
		t.Fatalf("got = %+v, want Crisis marked failed", got)
	}
}

func TestRecordDownload_RetrySuccessClearsFailed(t *testing.T) {
	s := newTestStore(t)
	md5 := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if err := s.RecordDownload(DownloadRecord{MD5: md5, Title: "Crisis", Failed: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{MD5: md5, Title: "Crisis", Failed: false}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDownloads(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Failed {
		t.Fatalf("got = %+v, want the retry to clear failed", got)
	}
}
