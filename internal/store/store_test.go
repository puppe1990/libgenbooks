package store

import (
	"errors"
	"testing"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStore_Migrations(t *testing.T) {
	s := newTestStore(t)

	for _, table := range []string{"users", "sessions"} {
		var name string
		err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name = ?", table).Scan(&name)
		if err != nil {
			t.Fatalf("%s table not found: %v", table, err)
		}
	}
}

func TestStore_CreateUser_ThenFind(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.CreateUser("alice@example.com", "hash"); err != nil {
		t.Fatal(err)
	}

	found, err := s.FindUserByEmail("alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if found.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", found.Email)
	}

	if _, err := s.CreateUser("alice@example.com", "hash"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("duplicate CreateUser err = %v, want ErrEmailTaken", err)
	}
}

func TestEnsureUser_CreatesOnceWithoutOverwritingHash(t *testing.T) {
	s := newTestStore(t)

	id1, err := s.EnsureUser("Owner@Example.com", "hash-a")
	if err != nil {
		t.Fatal(err)
	}
	if id1 < 1 {
		t.Fatalf("id = %d", id1)
	}
	id2, err := s.EnsureUser("owner@example.com", "hash-b")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("second EnsureUser id = %d, want %d", id2, id1)
	}
	found, err := s.FindUserByEmail("OWNER@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != id1 {
		t.Fatalf("FindUserByEmail id = %d, want %d", found.ID, id1)
	}
	if found.Email != "owner@example.com" {
		t.Errorf("Email = %q, want lowercase owner@example.com", found.Email)
	}
	if found.PasswordHash != "hash-a" {
		t.Errorf("PasswordHash = %q, want hash-a (must not overwrite)", found.PasswordHash)
	}
}

func TestEnsureUser_RejectsEmptyEmail(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.EnsureUser("  ", "hash"); err == nil {
		t.Fatal("expected error for empty email")
	}
}
