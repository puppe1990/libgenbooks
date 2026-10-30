package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/devlog"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
	caissqlite "github.com/puppe1990/amarra-cais/pkg/cais/sqlite"
	"github.com/puppe1990/amarra-cais/pkg/cais/sqllog"
	_ "modernc.org/sqlite"

	"github.com/puppe1990/libgenbooks/internal/models"
)

var ErrEmailTaken = errors.New("email already registered")

type Store interface {
	FindUserByEmail(email string) (models.User, error)
	CreateUser(email, passwordHash string) (int64, error)
	EnsureUser(email, passwordHash string) (int64, error)
	CreatePasswordResetToken(userID int64) (string, error)
	ClearPasswordResetTokens(userID int64) error
	FindPasswordResetUserID(token string) (int64, bool)
	ResetPasswordWithToken(token, passwordHash string) error
	Sessions() session.Store
	Ping() error
	DB() *sql.DB
	Close() error
}

type SQLiteStore struct {
	db *sqllog.DB
}

func NewSQLiteStore(dsn string, env string) (*SQLiteStore, error) {
	if dsn != ":memory:" {
		dir := filepath.Dir(dsn)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", caissqlite.DSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := caissqlite.Configure(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}

	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	cfg := sqllog.ConfigForEnv(env)
	if cfg.Enabled {
		cfg.Writer = devlog.MirrorDefault(os.Stdout)
	}
	wrapped := sqllog.Wrap(db, cfg)
	if err := seedAuthData(wrapped.Raw(), env); err != nil {
		_ = wrapped.Close()
		return nil, err
	}
	return &SQLiteStore{db: wrapped}, nil
}

func seedAuthData(db *sql.DB, env string) error {
	if env != "development" {
		return nil
	}
	if err := session.EnsureSQLiteSchema(db); err != nil {
		return err
	}
	hash, err := session.HashPassword("password")
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT OR IGNORE INTO users (email, password_hash, is_admin) VALUES (?, ?, 1)", "demo@example.com", hash)
	return err
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *SQLiteStore) FindUserByEmail(email string) (models.User, error) {
	email = normalizeEmail(email)
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, password_hash, created_at FROM users WHERE lower(email) = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return models.User{}, fmt.Errorf("find user: %w", err)
	}
	return u, nil
}

func (s *SQLiteStore) CreateUser(email, passwordHash string) (int64, error) {
	email = normalizeEmail(email)
	if email == "" {
		return 0, fmt.Errorf("create user: empty email")
	}
	result, err := s.db.Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		email, passwordHash,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrEmailTaken
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return result.LastInsertId()
}

// EnsureUser inserts the email when missing and returns the row id.
// A second call with the same email keeps the original password_hash.
func (s *SQLiteStore) EnsureUser(email, passwordHash string) (int64, error) {
	email = normalizeEmail(email)
	if email == "" {
		return 0, fmt.Errorf("ensure user: empty email")
	}
	if _, err := s.db.Exec(
		"INSERT OR IGNORE INTO users (email, password_hash) VALUES (?, ?)",
		email, passwordHash,
	); err != nil {
		return 0, fmt.Errorf("ensure user %s: %w", email, err)
	}
	u, err := s.FindUserByEmail(email)
	if err != nil {
		return 0, err
	}
	return u.ID, nil
}

func (s *SQLiteStore) Sessions() session.Store {
	return session.NewSQLiteStore(s.db.Raw())
}

func (s *SQLiteStore) Ping() error {
	return s.db.Raw().Ping()
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db.Raw()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
