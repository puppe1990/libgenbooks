package handlers

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/passwordreset"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/store"
)

type captureNotifier struct {
	emails []string
	tokens []string
}

func (c *captureNotifier) NotifyReset(email, token string) error {
	c.emails = append(c.emails, email)
	c.tokens = append(c.tokens, token)
	return nil
}

type failingNotifier struct {
	email string
	token string
}

func (f *failingNotifier) NotifyReset(email, token string) error {
	f.email = email
	f.token = token
	return errors.New("smtp down")
}

func newAuthHandlerForReset(t *testing.T, s store.Store, notify passwordreset.Notifier) *AuthHandler {
	t.Helper()
	h := NewAuthHandler(setupTestViews(t), s, testSite(), s.Sessions(), cais.Config{AppURL: "http://localhost:8080"}, appi18n.DefaultCatalog())
	h.resetNotify = notify
	return h
}

func TestAuth_ForgotPasswordPost_unknownEmail_showsSameMessage(t *testing.T) {
	s := setupTestStore(t)
	notify := &captureNotifier{}
	h := newAuthHandlerForReset(t, s, notify)

	form := url.Values{"email": {"missing@example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ForgotPasswordPost(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
	if len(notify.emails) != 0 {
		t.Fatal("should not notify for unknown email")
	}
}

func TestAuth_ForgotPasswordPost_knownEmail_notifiesAndRedirects(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:", "development")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	notify := &captureNotifier{}
	h := newAuthHandlerForReset(t, s, notify)

	form := url.Values{"email": {"demo@example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ForgotPasswordPost(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
	if len(notify.emails) != 1 || notify.emails[0] != "demo@example.com" {
		t.Fatalf("notify emails = %v", notify.emails)
	}
	if len(notify.tokens) != 1 || notify.tokens[0] == "" {
		t.Fatalf("notify tokens = %v", notify.tokens)
	}
}

// #223: a delivery failure must stay observably identical to a missing account
// (no enumeration) while logging the failure and invalidating the token.
func TestAuth_ForgotPasswordPost_deliveryFailure_logsAndInvalidatesToken(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:", "development")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	notify := &failingNotifier{}
	h := newAuthHandlerForReset(t, s, notify)

	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	form := url.Values{"email": {"demo@example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ForgotPasswordPost(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 (must match missing account)", rr.Code)
	}
	if notify.token == "" {
		t.Fatal("notifier must receive the token")
	}
	if !strings.Contains(logs.String(), "password reset") {
		t.Errorf("delivery failure must be logged server-side, got %q", logs.String())
	}
	if strings.Contains(logs.String(), notify.token) {
		t.Error("log must never contain the reset token")
	}
	if strings.Contains(rr.Body.String(), notify.token) {
		t.Error("response must never contain the reset token")
	}
	if _, ok := s.FindPasswordResetUserID(notify.token); ok {
		t.Fatal("token must be invalidated when delivery fails")
	}
}

func TestAuth_ForgotPassword_RendersForm(t *testing.T) {
	s := setupTestStore(t)
	h := newAuthHandlerForReset(t, s, &captureNotifier{})

	req := httptest.NewRequest(http.MethodGet, "/forgot-password", nil)
	rr := httptest.NewRecorder()
	h.ForgotPassword(rr, req)

	if !strings.Contains(rr.Body.String(), `action="/forgot-password"`) {
		t.Errorf("missing forgot-password form, got: %s", rr.Body.String())
	}
}

func TestAuth_ResetPasswordPost_validToken_updatesPassword(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:", "development")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	user, err := s.FindUserByEmail("demo@example.com")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CreatePasswordResetToken(user.ID)
	if err != nil {
		t.Fatal(err)
	}

	h := newAuthHandlerForReset(t, s, &captureNotifier{})
	form := url.Values{
		"token":                 {token},
		"password":              {"new-password-123"},
		"password_confirmation": {"new-password-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ResetPasswordPost(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/login" {
		t.Errorf("Location = %q, want /login", rr.Header().Get("Location"))
	}

	updated, err := s.FindUserByEmail("demo@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !session.VerifyPassword(updated.PasswordHash, "new-password-123") {
		t.Fatal("password was not updated")
	}
}

func TestAuth_ResetPasswordPost_invalidToken_rendersError(t *testing.T) {
	s := setupTestStore(t)
	h := newAuthHandlerForReset(t, s, &captureNotifier{})

	form := url.Values{
		"token":                 {"bad-token"},
		"password":              {"new-password-123"},
		"password_confirmation": {"new-password-123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ResetPasswordPost(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid or has expired") {
		t.Errorf("missing token error, got: %s", rr.Body.String())
	}
}
