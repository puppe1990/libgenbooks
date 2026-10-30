package store

import (
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

func TestFindUser_ReturnsCreatedRow(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateUser("reader@example.com", "hash-one")
	if err != nil {
		t.Fatal(err)
	}

	found, err := s.FindUser(id)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != id {
		t.Errorf("ID = %d, want %d", found.ID, id)
	}
	if found.Email != "reader@example.com" {
		t.Errorf("Email = %q, want reader@example.com", found.Email)
	}
	if found.PasswordHash != "hash-one" {
		t.Errorf("PasswordHash = %q, want hash-one", found.PasswordHash)
	}
}

func TestFindUser_MissingIDErrors(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.FindUser(99); err == nil {
		t.Fatal("expected error for missing user id")
	}
}

func TestChangePassword_UpdatesHashAndRevokesSessions(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateUser("reader@example.com", mustHash(t, "old-pass-1"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Sessions().Create(id)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ChangePassword(id, mustHash(t, "new-pass-99")); err != nil {
		t.Fatal(err)
	}

	found, err := s.FindUser(id)
	if err != nil {
		t.Fatal(err)
	}
	if !session.VerifyPassword(found.PasswordHash, "new-pass-99") {
		t.Fatal("password was not updated")
	}
	if session.VerifyPassword(found.PasswordHash, "old-pass-1") {
		t.Fatal("old password still verifies")
	}
	if _, ok := s.Sessions().Get(token); ok {
		t.Fatal("sessions must be revoked after a password change")
	}
}

func TestChangePassword_LeavesOtherUsersSessions(t *testing.T) {
	s := newTestStore(t)
	idA, err := s.CreateUser("a@example.com", mustHash(t, "pass-aaaa"))
	if err != nil {
		t.Fatal(err)
	}
	idB, err := s.CreateUser("b@example.com", mustHash(t, "pass-bbbb"))
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := s.Sessions().Create(idB)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ChangePassword(idA, mustHash(t, "pass-cccc")); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Sessions().Get(tokenB); !ok {
		t.Fatal("other user's session must remain")
	}
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := session.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
