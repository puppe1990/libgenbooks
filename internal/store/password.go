package store

import (
	"fmt"

	"github.com/puppe1990/libgenbooks/internal/models"
)

func (s *SQLiteStore) FindUser(id int64) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, password_hash, created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return models.User{}, fmt.Errorf("find user %d: %w", id, err)
	}
	return u, nil
}

// ChangePassword writes the new hash and drops open sessions, same as a reset
// token: the next request must sign in with the new password.
func (s *SQLiteStore) ChangePassword(id int64, passwordHash string) error {
	tx, err := s.db.Raw().Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, id); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit change password: %w", err)
	}
	return nil
}
