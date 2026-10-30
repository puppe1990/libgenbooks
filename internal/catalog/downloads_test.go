package catalog

import (
	"fmt"
	"testing"
	"time"
)

func seedDownloads(t *testing.T, s *Store, userID int64, count int, failed bool) {
	t.Helper()
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		if err := s.RecordDownload(DownloadRecord{
			UserID:       userID,
			MD5:          fmt.Sprintf("%032x", i+1),
			Title:        fmt.Sprintf("Book %02d", i+1),
			Failed:       failed,
			DownloadedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecordDownload_ListsNewestFirst(t *testing.T) {
	s := newTestStore(t)
	older := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	if err := s.RecordDownload(DownloadRecord{
		UserID: 1, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "Older", DownloadedAt: older,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{
		UserID: 1, MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Title: "Newer", Author: "Ada", Year: 2024, Extension: "epub", DownloadedAt: newer,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDownloads(1, 10)
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
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: md5, Title: "First", DownloadedAt: first}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: md5, Title: "Second", Author: "Bob", DownloadedAt: first.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDownloads(1, 10)
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
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: "nope", Title: "X"}); err == nil {
		t.Fatal("RecordDownload(nope) = nil, want error")
	}
}

func TestListDownloads_Empty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.ListDownloads(1, 10)
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
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: md5, Title: "Crisis", Failed: true}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDownloads(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Failed || got[0].Title != "Crisis" {
		t.Fatalf("got = %+v, want Crisis marked failed", got)
	}
}

func TestListFailedDownloads_OnlyFailedRows(t *testing.T) {
	s := newTestStore(t)
	if err := s.RecordDownload(DownloadRecord{
		UserID: 1, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "Ok",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{
		UserID: 1, MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Title: "Crisis", Failed: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDownloadsPage(1, 10, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Crisis" || !got[0].Failed {
		t.Fatalf("got = %+v, want only Crisis", got)
	}
}

func TestRecordDownload_RetrySuccessClearsFailed(t *testing.T) {
	s := newTestStore(t)
	md5 := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: md5, Title: "Crisis", Failed: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: md5, Title: "Crisis", Failed: false}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDownloads(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Failed {
		t.Fatalf("got = %+v, want the retry to clear failed", got)
	}
}

func TestRecordDownload_IsolatesPerUser(t *testing.T) {
	s := newTestStore(t)
	md5 := "ffffffffffffffffffffffffffffffff"
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: md5, Title: "Mine"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{UserID: 2, MD5: md5, Title: "Theirs"}); err != nil {
		t.Fatal(err)
	}

	mine, err := s.ListDownloads(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].Title != "Mine" || mine[0].UserID != 1 {
		t.Fatalf("user 1 = %+v, want only Mine", mine)
	}
	theirs, err := s.ListDownloads(2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 1 || theirs[0].Title != "Theirs" || theirs[0].UserID != 2 {
		t.Fatalf("user 2 = %+v, want only Theirs", theirs)
	}
}

func TestRecordDownload_RejectsMissingUser(t *testing.T) {
	s := newTestStore(t)
	if err := s.RecordDownload(DownloadRecord{MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "X"}); err == nil {
		t.Fatal("RecordDownload without UserID = nil, want error")
	}
}

func TestListFailedDownloads_HidesOtherUsers(t *testing.T) {
	s := newTestStore(t)
	if err := s.RecordDownload(DownloadRecord{
		UserID: 1, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Title: "Mine", Failed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{
		UserID: 2, MD5: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Title: "Theirs", Failed: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDownloadsPage(1, 10, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Mine" {
		t.Fatalf("got = %+v, want only Mine", got)
	}
}

func TestListDownloadsPage_SlicesNewestFirst(t *testing.T) {
	s := newTestStore(t)
	seedDownloads(t, s, 1, 25, false)

	first, err := s.ListDownloadsPage(1, 10, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 10 || first[0].Title != "Book 25" || first[9].Title != "Book 16" {
		t.Fatalf("first page = %d rows starting %q, want Book 25..Book 16", len(first), first[0].Title)
	}

	last, err := s.ListDownloadsPage(1, 10, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 5 || last[0].Title != "Book 05" || last[4].Title != "Book 01" {
		t.Fatalf("last page = %d rows, want the 5 oldest", len(last))
	}
}

func TestCountDownloads_ScopesUserAndFilter(t *testing.T) {
	s := newTestStore(t)
	seedDownloads(t, s, 1, 3, false)
	seedDownloads(t, s, 2, 5, true)

	all, err := s.CountDownloads(1, false)
	if err != nil {
		t.Fatal(err)
	}
	if all != 3 {
		t.Fatalf("CountDownloads(1, all) = %d, want 3", all)
	}
	failed, err := s.CountDownloads(1, true)
	if err != nil {
		t.Fatal(err)
	}
	if failed != 0 {
		t.Fatalf("CountDownloads(1, failed) = %d, want 0", failed)
	}
	theirs, err := s.CountDownloads(2, true)
	if err != nil {
		t.Fatal(err)
	}
	if theirs != 5 {
		t.Fatalf("CountDownloads(2, failed) = %d, want 5", theirs)
	}
}

func TestDeleteDownload_RemovesOnlyOwnRow(t *testing.T) {
	s := newTestStore(t)
	md5 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := s.RecordDownload(DownloadRecord{UserID: 1, MD5: md5, Title: "Mine"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDownload(DownloadRecord{UserID: 2, MD5: md5, Title: "Theirs"}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteDownload(1, md5); err != nil {
		t.Fatal(err)
	}

	mine, err := s.ListDownloads(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 0 {
		t.Fatalf("user 1 history = %+v, want empty", mine)
	}
	theirs, err := s.ListDownloads(2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 1 || theirs[0].Title != "Theirs" {
		t.Fatalf("user 2 history = %+v, want Theirs untouched", theirs)
	}
}

func TestDeleteDownload_RejectsBadInput(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteDownload(0, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("DeleteDownload without user = nil, want error")
	}
	if err := s.DeleteDownload(1, "nope"); err == nil {
		t.Fatal("DeleteDownload(nope) = nil, want error")
	}
}

func TestClaimOrphanDownloads_AssignsUserZeroRows(t *testing.T) {
	s := newTestStore(t)
	_, err := s.DB().Exec(
		`INSERT INTO downloads (user_id, md5, title, author, year, extension, filesize, downloaded_at, failed)
		 VALUES (0, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'Alice', '', 0, 'fb2', 0, '2026-10-30T13:39:16Z', 0)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimOrphanDownloads(9); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDownloads(9, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Alice" || got[0].UserID != 9 {
		t.Fatalf("got = %+v, want Alice claimed by user 9", got)
	}
	orphans, err := s.ListDownloads(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans left = %+v", orphans)
	}
}
