package app

import (
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/store"
)

func TestAssignHistoryOwner_ClaimsOrphans(t *testing.T) {
	users, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Close() })
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })
	if _, err := books.DB().Exec(
		`INSERT INTO downloads (user_id, md5, title, author, year, extension, filesize, downloaded_at, failed)
		 VALUES (0, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'Alice', '', 0, 'fb2', 0, '2026-10-30T13:39:16Z', 0)`,
	); err != nil {
		t.Fatal(err)
	}

	id, err := AssignHistoryOwner(users, books, "matheus.puppe@gmail.com", "password12")
	if err != nil {
		t.Fatal(err)
	}
	if id < 1 {
		t.Fatalf("owner id = %d", id)
	}

	got, err := books.ListDownloads(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Alice" || got[0].UserID != id {
		t.Fatalf("got = %+v, want Alice claimed by owner", got)
	}
	user, err := users.FindUserByEmail("matheus.puppe@gmail.com")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != id {
		t.Fatalf("FindUserByEmail id = %d, want %d", user.ID, id)
	}
}

func TestAssignHistoryOwner_EmptyEmailNoOp(t *testing.T) {
	users, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Close() })
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })

	id, err := AssignHistoryOwner(users, books, "", "password12")
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0", id)
	}
}

func TestAssignHistoryOwner_ExistingUserSkipsPassword(t *testing.T) {
	users, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Close() })
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })
	hash, err := session.HashPassword("kept-secret")
	if err != nil {
		t.Fatal(err)
	}
	wantID, err := users.CreateUser("matheus.puppe@gmail.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := books.DB().Exec(
		`INSERT INTO downloads (user_id, md5, title, author, year, extension, filesize, downloaded_at, failed)
		 VALUES (0, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'Crisis', '', 2026, 'epub', 0, '2026-10-30T14:25:00Z', 1)`,
	); err != nil {
		t.Fatal(err)
	}

	id, err := AssignHistoryOwner(users, books, "Matheus.Puppe@gmail.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if id != wantID {
		t.Fatalf("id = %d, want %d", id, wantID)
	}
	user, err := users.FindUserByEmail("matheus.puppe@gmail.com")
	if err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash != hash {
		t.Fatal("must not overwrite an existing password hash")
	}
	got, err := books.ListDownloads(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Crisis" {
		t.Fatalf("got = %+v, want Crisis claimed", got)
	}
}

func TestAssignHistoryOwner_MissingUserNeedsPassword(t *testing.T) {
	users, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Close() })
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })

	_, err = AssignHistoryOwner(users, books, "matheus.puppe@gmail.com", "")
	if err == nil {
		t.Fatal("expected error when owner is missing and password is empty")
	}
}
