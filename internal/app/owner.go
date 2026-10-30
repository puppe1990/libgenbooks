package app

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	"github.com/puppe1990/libgenbooks/internal/store"
)

// AssignHistoryOwner creates the cutover account when missing and moves
// leftover user_id=0 download rows onto that user.
func AssignHistoryOwner(users store.Store, books *catalog.Store, email, password string) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, nil
	}
	if books == nil {
		return 0, fmt.Errorf("assign history owner: catalog is required")
	}
	userID, err := lookupOrCreateOwner(users, email, password)
	if err != nil {
		return 0, err
	}
	if err := books.ClaimOrphanDownloads(userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func lookupOrCreateOwner(users store.Store, email, password string) (int64, error) {
	user, err := users.FindUserByEmail(email)
	if err == nil {
		return user.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if strings.TrimSpace(password) == "" {
		return 0, fmt.Errorf("history owner %q does not exist; set LIBGEN_HISTORY_OWNER_PASSWORD", email)
	}
	hash, err := session.HashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("hash history owner password: %w", err)
	}
	return users.EnsureUser(email, hash)
}
