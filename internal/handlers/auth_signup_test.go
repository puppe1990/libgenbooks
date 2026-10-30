package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/middleware"

	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/store"
)

func newAuthHandlerForSignup(t *testing.T) (*AuthHandler, store.Store) {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:", "development")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	h := NewAuthHandler(setupTestViews(t), s, testSite(), s.Sessions(), cais.Config{}, appi18n.DefaultCatalog())
	return h, s
}

func TestAuth_SignUpPost_createsUserAndRedirects(t *testing.T) {
	h, s := newAuthHandlerForSignup(t)

	form := url.Values{}
	form.Set("email", "signup@example.com")
	form.Set("password", "password123")
	form.Set("password_confirmation", "password123")
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.SignUpPost(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/" {
		t.Errorf("Location = %q, want /", rr.Header().Get("Location"))
	}

	user, err := s.FindUserByEmail("signup@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID == 0 {
		t.Fatal("user id = 0")
	}

	protected := middleware.LoadSession(s.Sessions())(middleware.RequireAdmin("/login", s.DB())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	cookies := rr.Result().Cookies()
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		attempt := httptest.NewRequest(method, "/admin/items", nil)
		for _, cookie := range cookies {
			attempt.AddCookie(cookie)
		}
		denied := httptest.NewRecorder()
		protected.ServeHTTP(denied, attempt)
		if denied.Code != http.StatusForbidden {
			t.Fatalf("signup user %s: status %d, want 403", method, denied.Code)
		}
	}
	if _, err := s.DB().Exec("UPDATE users SET is_admin = 1 WHERE id = ?", user.ID); err != nil {
		t.Fatal(err)
	}
	attempt := httptest.NewRequest("GET", "/admin/items", nil)
	for _, cookie := range cookies {
		attempt.AddCookie(cookie)
	}
	allowed := httptest.NewRecorder()
	protected.ServeHTTP(allowed, attempt)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("admin status = %d", allowed.Code)
	}
}

func TestAuth_SignUpPost_duplicateEmail_returnsError(t *testing.T) {
	h, _ := newAuthHandlerForSignup(t)

	form := url.Values{}
	form.Set("email", "signup@example.com")
	form.Set("password", "password123")
	form.Set("password_confirmation", "password123")
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.SignUpPost(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("first signup status = %d, want 303", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr2 := httptest.NewRecorder()
	h.SignUpPost(rr2, req2)
	if rr2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate signup status = %d, want 422", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "already registered") {
		t.Errorf("missing email taken error, got: %s", rr2.Body.String())
	}
}

func TestAuth_SignUp_RendersForm(t *testing.T) {
	h, _ := newAuthHandlerForSignup(t)

	req := httptest.NewRequest(http.MethodGet, "/signup", nil)
	rr := httptest.NewRecorder()
	h.SignUp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `action="/signup"`) {
		t.Errorf("missing signup form, got: %s", rr.Body.String())
	}
}
