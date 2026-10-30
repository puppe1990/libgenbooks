package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/store"
)

func newConfiguracoesHandler(t *testing.T) (*ConfiguracoesHandler, store.Store) {
	t.Helper()
	s := setupTestStore(t)
	h := NewConfiguracoesHandler(setupTestViews(t), s, testSite(), appi18n.DefaultCatalog(), cais.Config{})
	return h, s
}

func createSignedInReader(t *testing.T, s store.Store, password string) (int64, string) {
	t.Helper()
	hash, err := session.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateUser("reader@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Sessions().Create(id)
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}

func TestConfiguracoes_Get_RendersPasswordForm(t *testing.T) {
	h, _ := newConfiguracoesHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/configuracoes", nil)
	req = session.WithUserID(req, 1)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Settings") {
		t.Errorf("missing settings heading, got: %s", body)
	}
	for _, want := range []string{
		`name="current_password"`,
		`name="password"`,
		`name="password_confirmation"`,
		`action="/configuracoes"`,
		`data-amarra-skip`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s, got: %s", want, body)
		}
	}
	if strings.Contains(body, `href="/dashboard"`) {
		t.Errorf("settings page must not link to /dashboard, got: %s", body)
	}
}

func TestConfiguracoes_Get_includesFlash(t *testing.T) {
	h, _ := newConfiguracoesHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/configuracoes", nil)
	req = session.WithUserID(req, 1)
	req = flash.WithMessage(req, flash.Message{Kind: "notice", Message: "Welcome back!"})
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Welcome back!") {
		t.Errorf("missing flash notice, got: %s", rr.Body.String())
	}
}

func TestConfiguracoes_Get_signedInShowsAuthChrome(t *testing.T) {
	h, _ := newConfiguracoesHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/configuracoes", nil)
	req = session.WithUserID(req, 1)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `action="/logout"`) {
		t.Errorf("signed-in page should offer sign-out, got: %s", body)
	}
	if !strings.Contains(body, `href="/configuracoes"`) {
		t.Errorf("signed-in page should link to /configuracoes, got: %s", body)
	}
}

func TestConfiguracoes_Get_withoutSessionRedirectsLogin(t *testing.T) {
	h, _ := newConfiguracoesHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/configuracoes", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestConfiguracoes_Post_wrongCurrentPassword(t *testing.T) {
	h, s := newConfiguracoesHandler(t)
	id, _ := createSignedInReader(t, s, "old-pass-1")

	form := url.Values{
		"current_password":      {"nope-nope"},
		"password":              {"new-pass-99"},
		"password_confirmation": {"new-pass-99"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configuracoes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, id)
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Current password is incorrect.") {
		t.Errorf("missing wrong-password error, got: %s", rr.Body.String())
	}
}

func TestConfiguracoes_Post_shortPassword(t *testing.T) {
	h, s := newConfiguracoesHandler(t)
	id, _ := createSignedInReader(t, s, "old-pass-1")

	form := url.Values{
		"current_password":      {"old-pass-1"},
		"password":              {"short"},
		"password_confirmation": {"short"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configuracoes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, id)
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Password must be at least 8 characters.") {
		t.Errorf("missing too-short error, got: %s", rr.Body.String())
	}
}

func TestConfiguracoes_Post_mismatch(t *testing.T) {
	h, s := newConfiguracoesHandler(t)
	id, _ := createSignedInReader(t, s, "old-pass-1")

	form := url.Values{
		"current_password":      {"old-pass-1"},
		"password":              {"new-pass-99"},
		"password_confirmation": {"new-pass-00"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configuracoes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, id)
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Passwords do not match.") {
		t.Errorf("missing mismatch error, got: %s", rr.Body.String())
	}
}

func TestConfiguracoes_Post_successRevokesAndRedirectsLogin(t *testing.T) {
	h, s := newConfiguracoesHandler(t)
	id, token := createSignedInReader(t, s, "old-pass-1")

	form := url.Values{
		"current_password":      {"old-pass-1"},
		"password":              {"new-pass-99"},
		"password_confirmation": {"new-pass-99"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configuracoes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: session.DefaultCookieName, Value: token})
	req = session.WithUserID(req, id)
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
	if _, ok := s.Sessions().Get(token); ok {
		t.Fatal("session must be revoked after password change")
	}
	user, err := s.FindUser(id)
	if err != nil {
		t.Fatal(err)
	}
	if !session.VerifyPassword(user.PasswordHash, "new-pass-99") {
		t.Fatal("new password must verify")
	}
	foundFlash := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "cais_flash" && c.Value != "" {
			foundFlash = true
			break
		}
	}
	if !foundFlash {
		t.Error("password change must set cais_flash via flash.Set")
	}
}
